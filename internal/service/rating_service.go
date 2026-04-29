package service

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/walmart/ratings-reviews/internal/domain"
	"github.com/walmart/ratings-reviews/internal/repository"
)

type RatingService struct {
	reviews repository.ReviewRepository
	ratings repository.RatingRepository
	log     *slog.Logger
	now     func() time.Time
}

func NewRatingService(reviews repository.ReviewRepository, ratings repository.RatingRepository, log *slog.Logger) *RatingService {
	return &RatingService{reviews: reviews, ratings: ratings, log: log, now: time.Now}
}

// Recompute recalculates the aggregate for a product. In production this would
// be triggered by a debounced consumer of review.approved / review.deleted
// events to avoid hot-key contention on popular products.
func (s *RatingService) Recompute(ctx context.Context, productID string) error {
	items, err := s.reviews.AllByProduct(ctx, productID, []domain.ReviewStatus{domain.StatusApproved})
	if err != nil {
		return err
	}
	dist := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	sum := 0
	for _, rv := range items {
		dist[rv.Rating]++
		sum += rv.Rating
	}
	avg := 0.0
	if n := len(items); n > 0 {
		avg = math.Round(float64(sum)/float64(n)*100) / 100
	}
	return s.ratings.Upsert(ctx, &domain.RatingSummary{
		ProductID:     productID,
		AverageRating: avg,
		TotalReviews:  len(items),
		Distribution:  dist,
		UpdatedAt:     s.now(),
	})
}

func (s *RatingService) Get(ctx context.Context, productID string) (*domain.RatingSummary, error) {
	return s.ratings.Get(ctx, productID)
}
