package repository

import (
	"context"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/domain"
)

type ReviewListFilter struct {
	Statuses []domain.ReviewStatus
	Cursor   string
	Limit    int
}

type ReviewRepository interface {
	Create(ctx context.Context, r *domain.Review) error
	Get(ctx context.Context, id string) (*domain.Review, error)
	Update(ctx context.Context, r *domain.Review) error
	Delete(ctx context.Context, id string) error

	ListByProduct(ctx context.Context, productID string, f ReviewListFilter) (items []*domain.Review, nextCursor string, err error)
	ListByStatus(ctx context.Context, status domain.ReviewStatus, offset, limit int) (items []*domain.Review, total int, err error)
	AllByProduct(ctx context.Context, productID string, statuses []domain.ReviewStatus) ([]*domain.Review, error)

	IncrementHelpful(ctx context.Context, id string, delta int) error
	IncrementUnhelpful(ctx context.Context, id string, delta int) error

	// Idempotency: returns the previously-stored review id for this key, or "" if unseen.
	LookupIdempotencyKey(ctx context.Context, key string) (string, error)
	SaveIdempotencyKey(ctx context.Context, key, reviewID string) error
}

type RatingRepository interface {
	Get(ctx context.Context, productID string) (*domain.RatingSummary, error)
	Upsert(ctx context.Context, s *domain.RatingSummary) error
}
