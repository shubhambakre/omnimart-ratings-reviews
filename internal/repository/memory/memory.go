package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/domain"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/repository"
)

type ReviewRepo struct {
	mu        sync.RWMutex
	byID      map[string]*domain.Review
	byProduct map[string]map[string]struct{} // productID -> set of review IDs
	idempo    map[string]string              // idempotency key -> review id
}

func NewReviewRepo() *ReviewRepo {
	return &ReviewRepo{
		byID:      make(map[string]*domain.Review),
		byProduct: make(map[string]map[string]struct{}),
		idempo:    make(map[string]string),
	}
}

func (r *ReviewRepo) Create(_ context.Context, rv *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rv
	r.byID[rv.ID] = &cp
	if _, ok := r.byProduct[rv.ProductID]; !ok {
		r.byProduct[rv.ProductID] = make(map[string]struct{})
	}
	r.byProduct[rv.ProductID][rv.ID] = struct{}{}
	return nil
}

func (r *ReviewRepo) Get(_ context.Context, id string) (*domain.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rv, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrReviewNotFound
	}
	cp := *rv
	return &cp, nil
}

func (r *ReviewRepo) Update(_ context.Context, rv *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[rv.ID]; !ok {
		return domain.ErrReviewNotFound
	}
	cp := *rv
	r.byID[rv.ID] = &cp
	return nil
}

func (r *ReviewRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rv, ok := r.byID[id]
	if !ok {
		return domain.ErrReviewNotFound
	}
	delete(r.byID, id)
	if set, ok := r.byProduct[rv.ProductID]; ok {
		delete(set, id)
	}
	return nil
}

func (r *ReviewRepo) AllByProduct(_ context.Context, productID string, statuses []domain.ReviewStatus) ([]*domain.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byProduct[productID]
	out := make([]*domain.Review, 0, len(ids))
	for id := range ids {
		rv := r.byID[id]
		if rv == nil {
			continue
		}
		if !statusMatch(rv.Status, statuses) {
			continue
		}
		cp := *rv
		out = append(out, &cp)
	}
	return out, nil
}

func (r *ReviewRepo) ListByProduct(ctx context.Context, productID string, f repository.ReviewListFilter) ([]*domain.Review, string, error) {
	items, err := r.AllByProduct(ctx, productID, f.Statuses)
	if err != nil {
		return nil, "", err
	}
	// Stable order: newest first, ties broken by ID descending.
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	startIdx := 0
	if f.Cursor != "" {
		startIdx = repository.DecodeCursor(f.Cursor, items)
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	end := startIdx + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[startIdx:end]
	var next string
	if end < len(items) {
		next = repository.EncodeCursor(items[end-1])
	}
	return page, next, nil
}

func (r *ReviewRepo) ListByStatus(_ context.Context, status domain.ReviewStatus, offset, limit int) ([]*domain.Review, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	matched := make([]*domain.Review, 0)
	for _, rv := range r.byID {
		if rv.Status == status {
			cp := *rv
			matched = append(matched, &cp)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.Before(matched[j].CreatedAt) })
	total := len(matched)
	if offset > total {
		offset = total
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (r *ReviewRepo) IncrementHelpful(_ context.Context, id string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rv, ok := r.byID[id]
	if !ok {
		return domain.ErrReviewNotFound
	}
	rv.HelpfulCount += delta
	if rv.HelpfulCount < 0 {
		rv.HelpfulCount = 0
	}
	return nil
}

func (r *ReviewRepo) IncrementUnhelpful(_ context.Context, id string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rv, ok := r.byID[id]
	if !ok {
		return domain.ErrReviewNotFound
	}
	rv.UnhelpfulCount += delta
	if rv.UnhelpfulCount < 0 {
		rv.UnhelpfulCount = 0
	}
	return nil
}

func (r *ReviewRepo) LookupIdempotencyKey(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.idempo[key], nil
}

func (r *ReviewRepo) SaveIdempotencyKey(_ context.Context, key, reviewID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempo[key] = reviewID
	return nil
}

type RatingRepo struct {
	mu      sync.RWMutex
	summary map[string]*domain.RatingSummary
}

func NewRatingRepo() *RatingRepo {
	return &RatingRepo{summary: make(map[string]*domain.RatingSummary)}
}

func (r *RatingRepo) Get(_ context.Context, productID string) (*domain.RatingSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.summary[productID]
	if !ok {
		return domain.NewRatingSummary(productID), nil
	}
	cp := *s
	cp.Distribution = make(map[int]int, len(s.Distribution))
	for k, v := range s.Distribution {
		cp.Distribution[k] = v
	}
	return &cp, nil
}

func (r *RatingRepo) Upsert(_ context.Context, s *domain.RatingSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	cp.Distribution = make(map[int]int, len(s.Distribution))
	for k, v := range s.Distribution {
		cp.Distribution[k] = v
	}
	r.summary[s.ProductID] = &cp
	return nil
}

func statusMatch(s domain.ReviewStatus, allowed []domain.ReviewStatus) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == s {
			return true
		}
	}
	return false
}

