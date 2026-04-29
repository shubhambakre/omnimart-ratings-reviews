package domain

import (
	"errors"
	"time"
)

type ReviewStatus string

const (
	StatusPending  ReviewStatus = "PENDING"
	StatusApproved ReviewStatus = "APPROVED"
	StatusRejected ReviewStatus = "REJECTED"
)

var (
	ErrReviewNotFound      = errors.New("review not found")
	ErrInvalidRating       = errors.New("rating must be between 1 and 5")
	ErrEmptyBody           = errors.New("review body required")
	ErrTitleTooLong        = errors.New("title exceeds 120 chars")
	ErrBodyTooLong         = errors.New("body exceeds 4000 chars")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
	ErrInvalidStatus       = errors.New("invalid status transition")
	ErrUnauthorized        = errors.New("unauthorized")
)

type Review struct {
	ID               string       `json:"id"`
	ProductID        string       `json:"productId"`
	CustomerID       string       `json:"customerId"`
	OrderID          string       `json:"orderId,omitempty"`
	Rating           int          `json:"rating"`
	Title            string       `json:"title"`
	Body             string       `json:"body"`
	VerifiedPurchase bool         `json:"verifiedPurchase"`
	Status           ReviewStatus `json:"status"`
	HelpfulCount     int          `json:"helpfulCount"`
	UnhelpfulCount   int          `json:"unhelpfulCount"`
	ModerationNotes  string       `json:"moderationNotes,omitempty"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

func (r *Review) Validate() error {
	if r.Rating < 1 || r.Rating > 5 {
		return ErrInvalidRating
	}
	if len(r.Body) == 0 {
		return ErrEmptyBody
	}
	if len(r.Title) > 120 {
		return ErrTitleTooLong
	}
	if len(r.Body) > 4000 {
		return ErrBodyTooLong
	}
	return nil
}

type RatingSummary struct {
	ProductID     string      `json:"productId"`
	AverageRating float64     `json:"averageRating"`
	TotalReviews  int         `json:"totalReviews"`
	Distribution  map[int]int `json:"distribution"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

// NewRatingSummary returns a zeroed summary for productID with a pre-populated
// distribution map (all five star buckets present, count=0).
func NewRatingSummary(productID string) *RatingSummary {
	return &RatingSummary{
		ProductID:    productID,
		Distribution: map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	}
}
