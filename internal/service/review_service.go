package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/walmart/ratings-reviews/internal/domain"
	"github.com/walmart/ratings-reviews/internal/moderation"
	"github.com/walmart/ratings-reviews/internal/repository"
)

type SubmitReviewInput struct {
	ProductID        string
	CustomerID       string
	OrderID          string
	Rating           int
	Title            string
	Body             string
	VerifiedPurchase bool
	IdempotencyKey   string
}

type ModerationDecision struct {
	Approve bool
	Notes   string
}

type ReviewService struct {
	reviews   repository.ReviewRepository
	ratings   repository.RatingRepository
	moderator moderation.Moderator
	rating    *RatingService
	log       *slog.Logger
	now       func() time.Time
}

func NewReviewService(reviews repository.ReviewRepository, ratings repository.RatingRepository, mod moderation.Moderator, rating *RatingService, log *slog.Logger) *ReviewService {
	return &ReviewService{
		reviews:   reviews,
		ratings:   ratings,
		moderator: mod,
		rating:    rating,
		log:       log,
		now:       time.Now,
	}
}

func (s *ReviewService) Submit(ctx context.Context, in SubmitReviewInput) (*domain.Review, error) {
	if in.IdempotencyKey != "" {
		if existing, _ := s.reviews.LookupIdempotencyKey(ctx, in.IdempotencyKey); existing != "" {
			return s.reviews.Get(ctx, existing)
		}
	}

	rv := &domain.Review{
		ID:               newID(),
		ProductID:        in.ProductID,
		CustomerID:       in.CustomerID,
		OrderID:          in.OrderID,
		Rating:           in.Rating,
		Title:            in.Title,
		Body:             in.Body,
		VerifiedPurchase: in.VerifiedPurchase,
		CreatedAt:        s.now(),
		UpdatedAt:        s.now(),
	}
	if err := rv.Validate(); err != nil {
		return nil, err
	}

	d := s.moderator.Inspect(rv.Title, rv.Body)
	if d.Clean {
		rv.Status = domain.StatusApproved
	} else {
		rv.Status = domain.StatusPending
		rv.ModerationNotes = d.Reason
	}

	if err := s.reviews.Create(ctx, rv); err != nil {
		return nil, err
	}
	if in.IdempotencyKey != "" {
		_ = s.reviews.SaveIdempotencyKey(ctx, in.IdempotencyKey, rv.ID)
	}

	if rv.Status == domain.StatusApproved {
		if err := s.rating.Recompute(ctx, rv.ProductID); err != nil {
			s.log.Error("recompute failed", "productId", rv.ProductID, "err", err)
		}
	}
	s.log.Info("review submitted", "id", rv.ID, "productId", rv.ProductID, "status", rv.Status)
	return rv, nil
}

func (s *ReviewService) Get(ctx context.Context, id string, includeUnpublished bool) (*domain.Review, error) {
	rv, err := s.reviews.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !includeUnpublished && rv.Status != domain.StatusApproved {
		return nil, domain.ErrReviewNotFound
	}
	return rv, nil
}

func (s *ReviewService) ListPublic(ctx context.Context, productID, cursor string, limit int) ([]*domain.Review, string, error) {
	return s.reviews.ListByProduct(ctx, productID, repository.ReviewListFilter{
		Statuses: []domain.ReviewStatus{domain.StatusApproved},
		Cursor:   cursor,
		Limit:    limit,
	})
}

func (s *ReviewService) ListPendingModeration(ctx context.Context, offset, limit int) ([]*domain.Review, int, error) {
	return s.reviews.ListByStatus(ctx, domain.StatusPending, offset, limit)
}

func (s *ReviewService) Moderate(ctx context.Context, id string, dec ModerationDecision) (*domain.Review, error) {
	rv, err := s.reviews.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rv.Status != domain.StatusPending {
		return nil, domain.ErrInvalidStatus
	}
	if dec.Approve {
		rv.Status = domain.StatusApproved
	} else {
		rv.Status = domain.StatusRejected
	}
	rv.ModerationNotes = dec.Notes
	rv.UpdatedAt = s.now()
	if err := s.reviews.Update(ctx, rv); err != nil {
		return nil, err
	}
	if rv.Status == domain.StatusApproved {
		if err := s.rating.Recompute(ctx, rv.ProductID); err != nil {
			s.log.Error("recompute failed", "productId", rv.ProductID, "err", err)
		}
	}
	return rv, nil
}

func (s *ReviewService) Delete(ctx context.Context, id string) error {
	rv, err := s.reviews.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.reviews.Delete(ctx, id); err != nil {
		return err
	}
	if rv.Status == domain.StatusApproved {
		if err := s.rating.Recompute(ctx, rv.ProductID); err != nil {
			s.log.Error("recompute failed", "productId", rv.ProductID, "err", err)
		}
	}
	return nil
}

func (s *ReviewService) Vote(ctx context.Context, id string, helpful bool) (*domain.Review, error) {
	if helpful {
		if err := s.reviews.IncrementHelpful(ctx, id, 1); err != nil {
			return nil, err
		}
	} else {
		if err := s.reviews.IncrementUnhelpful(ctx, id, 1); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, id, false)
}

type BulkResult struct {
	Created  []string `json:"created"`
	Failed   []string `json:"failed"`
	Messages []string `json:"messages,omitempty"`
}

func (s *ReviewService) BulkIngest(ctx context.Context, items []SubmitReviewInput) BulkResult {
	res := BulkResult{}
	for _, in := range items {
		rv, err := s.Submit(ctx, in)
		if err != nil {
			res.Failed = append(res.Failed, in.ProductID)
			res.Messages = append(res.Messages, err.Error())
			continue
		}
		res.Created = append(res.Created, rv.ID)
	}
	return res
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "rv_" + hex.EncodeToString(b)
}
