// Package sqlite provides SQLite-backed implementations of the repository interfaces.
// It uses modernc.org/sqlite (pure Go, no CGO) so it compiles on any platform
// without needing a system SQLite installation.
//
// Schema design notes (production alignment):
//   - reviews table: productID + status indexed separately for the two hot queries
//     (public read: productID+APPROVED; internal queue: status=PENDING)
//   - rating_summaries table: single row per product, upserted on every approval
//   - idempotency_keys table: unique key -> review_id, used for deduplicated writes
//
// All timestamps are stored as Unix nanoseconds (int64) for easy Go round-tripping.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/domain"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/repository"
)

// -----------------------------------------------------------------------
// Schema
// -----------------------------------------------------------------------

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS reviews (
    id                TEXT    PRIMARY KEY,
    product_id        TEXT    NOT NULL,
    customer_id       TEXT    NOT NULL,
    order_id          TEXT    NOT NULL DEFAULT '',
    rating            INTEGER NOT NULL,
    title             TEXT    NOT NULL DEFAULT '',
    body              TEXT    NOT NULL,
    verified_purchase INTEGER NOT NULL DEFAULT 0,
    status            TEXT    NOT NULL DEFAULT 'PENDING',
    helpful_count     INTEGER NOT NULL DEFAULT 0,
    unhelpful_count   INTEGER NOT NULL DEFAULT 0,
    moderation_notes  TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reviews_product_status ON reviews (product_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_status_created ON reviews (status, created_at ASC);

CREATE TABLE IF NOT EXISTS rating_summaries (
    product_id     TEXT    PRIMARY KEY,
    average_rating REAL    NOT NULL DEFAULT 0,
    total_reviews  INTEGER NOT NULL DEFAULT 0,
    distribution   TEXT    NOT NULL DEFAULT '{}',
    updated_at     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    idem_key  TEXT PRIMARY KEY,
    review_id TEXT NOT NULL
);
`

// -----------------------------------------------------------------------
// ReviewRepo
// -----------------------------------------------------------------------

type ReviewRepo struct {
	db *sql.DB
}

// NewReviewRepo opens (or creates) a SQLite file at path and runs the schema migration.
func NewReviewRepo(path string) (*ReviewRepo, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// Single writer to avoid SQLITE_BUSY under concurrent writes; reads use WAL.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	return &ReviewRepo{db: db}, nil
}

func (r *ReviewRepo) Create(_ context.Context, rv *domain.Review) error {
	_, err := r.db.Exec(`
		INSERT INTO reviews
		  (id, product_id, customer_id, order_id, rating, title, body,
		   verified_purchase, status, helpful_count, unhelpful_count,
		   moderation_notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rv.ID, rv.ProductID, rv.CustomerID, rv.OrderID,
		rv.Rating, rv.Title, rv.Body,
		boolInt(rv.VerifiedPurchase), string(rv.Status),
		rv.HelpfulCount, rv.UnhelpfulCount, rv.ModerationNotes,
		rv.CreatedAt.UnixNano(), rv.UpdatedAt.UnixNano(),
	)
	return err
}

func (r *ReviewRepo) Get(_ context.Context, id string) (*domain.Review, error) {
	row := r.db.QueryRow(`SELECT * FROM reviews WHERE id = ?`, id)
	rv, err := scanReview(row)
	if err == sql.ErrNoRows {
		return nil, domain.ErrReviewNotFound
	}
	return rv, err
}

func (r *ReviewRepo) Update(_ context.Context, rv *domain.Review) error {
	res, err := r.db.Exec(`
		UPDATE reviews SET
		  product_id=?, customer_id=?, order_id=?, rating=?, title=?, body=?,
		  verified_purchase=?, status=?, helpful_count=?, unhelpful_count=?,
		  moderation_notes=?, updated_at=?
		WHERE id=?`,
		rv.ProductID, rv.CustomerID, rv.OrderID, rv.Rating, rv.Title, rv.Body,
		boolInt(rv.VerifiedPurchase), string(rv.Status),
		rv.HelpfulCount, rv.UnhelpfulCount, rv.ModerationNotes,
		rv.UpdatedAt.UnixNano(), rv.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

func (r *ReviewRepo) Delete(_ context.Context, id string) error {
	res, err := r.db.Exec(`DELETE FROM reviews WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

func (r *ReviewRepo) AllByProduct(_ context.Context, productID string, statuses []domain.ReviewStatus) ([]*domain.Review, error) {
	q, args := buildAllByProductQuery(productID, statuses)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReviews(rows)
}

func (r *ReviewRepo) ListByProduct(ctx context.Context, productID string, f repository.ReviewListFilter) ([]*domain.Review, string, error) {
	items, err := r.AllByProduct(ctx, productID, f.Statuses)
	if err != nil {
		return nil, "", err
	}
	// Reuse the same in-memory cursor logic as the memory repo — the sorted slice
	// is small enough (product page reads) that this is fine.
	startIdx := 0
	if f.Cursor != "" {
		startIdx = decodeCursor(f.Cursor, items)
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
		next = encodeCursor(items[end-1])
	}
	return page, next, nil
}

func (r *ReviewRepo) ListByStatus(_ context.Context, status domain.ReviewStatus, offset, limit int) ([]*domain.Review, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE status=?`, string(status)).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(`
		SELECT * FROM reviews WHERE status=? ORDER BY created_at ASC LIMIT ? OFFSET ?`,
		string(status), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanReviews(rows)
	return items, total, err
}

func (r *ReviewRepo) IncrementHelpful(_ context.Context, id string, delta int) error {
	res, err := r.db.Exec(`
		UPDATE reviews SET
		  helpful_count = MAX(0, helpful_count + ?),
		  updated_at = ?
		WHERE id = ?`, delta, time.Now().UnixNano(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

func (r *ReviewRepo) IncrementUnhelpful(_ context.Context, id string, delta int) error {
	res, err := r.db.Exec(`
		UPDATE reviews SET
		  unhelpful_count = MAX(0, unhelpful_count + ?),
		  updated_at = ?
		WHERE id = ?`, delta, time.Now().UnixNano(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

func (r *ReviewRepo) LookupIdempotencyKey(_ context.Context, key string) (string, error) {
	var id string
	err := r.db.QueryRow(`SELECT review_id FROM idempotency_keys WHERE idem_key=?`, key).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *ReviewRepo) SaveIdempotencyKey(_ context.Context, key, reviewID string) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO idempotency_keys (idem_key, review_id) VALUES (?,?)`, key, reviewID)
	return err
}

// -----------------------------------------------------------------------
// RatingRepo
// -----------------------------------------------------------------------

type RatingRepo struct {
	db *sql.DB
}

// NewRatingRepo shares the same DB file as ReviewRepo.
func NewRatingRepo(db *sql.DB) *RatingRepo {
	return &RatingRepo{db: db}
}

func (r *RatingRepo) Get(_ context.Context, productID string) (*domain.RatingSummary, error) {
	var dist string
	var avg float64
	var total int
	var updatedNano int64
	err := r.db.QueryRow(`
		SELECT average_rating, total_reviews, distribution, updated_at
		FROM rating_summaries WHERE product_id=?`, productID).
		Scan(&avg, &total, &dist, &updatedNano)
	if err == sql.ErrNoRows {
		return &domain.RatingSummary{
			ProductID:    productID,
			Distribution: map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	distMap := map[int]int{}
	_ = json.Unmarshal([]byte(dist), &distMap)
	return &domain.RatingSummary{
		ProductID:     productID,
		AverageRating: avg,
		TotalReviews:  total,
		Distribution:  distMap,
		UpdatedAt:     time.Unix(0, updatedNano),
	}, nil
}

func (r *RatingRepo) Upsert(_ context.Context, s *domain.RatingSummary) error {
	distBytes, _ := json.Marshal(s.Distribution)
	_, err := r.db.Exec(`
		INSERT INTO rating_summaries (product_id, average_rating, total_reviews, distribution, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(product_id) DO UPDATE SET
		  average_rating=excluded.average_rating,
		  total_reviews=excluded.total_reviews,
		  distribution=excluded.distribution,
		  updated_at=excluded.updated_at`,
		s.ProductID, s.AverageRating, s.TotalReviews, string(distBytes), s.UpdatedAt.UnixNano(),
	)
	return err
}

// DB returns the underlying *sql.DB so a shared connection can be passed to RatingRepo.
func (r *ReviewRepo) DB() *sql.DB { return r.db }

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func buildAllByProductQuery(productID string, statuses []domain.ReviewStatus) (string, []any) {
	args := []any{productID}
	q := `SELECT * FROM reviews WHERE product_id=?`
	if len(statuses) > 0 {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = placeholders[:len(placeholders)-1]
		q += " AND status IN (" + placeholders + ")"
		for _, s := range statuses {
			args = append(args, string(s))
		}
	}
	q += ` ORDER BY created_at DESC, id DESC`
	return q, args
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReview(s scanner) (*domain.Review, error) {
	var rv domain.Review
	var status string
	var vp int
	var createdNano, updatedNano int64
	err := s.Scan(
		&rv.ID, &rv.ProductID, &rv.CustomerID, &rv.OrderID,
		&rv.Rating, &rv.Title, &rv.Body,
		&vp, &status,
		&rv.HelpfulCount, &rv.UnhelpfulCount, &rv.ModerationNotes,
		&createdNano, &updatedNano,
	)
	if err != nil {
		return nil, err
	}
	rv.VerifiedPurchase = vp == 1
	rv.Status = domain.ReviewStatus(status)
	rv.CreatedAt = time.Unix(0, createdNano)
	rv.UpdatedAt = time.Unix(0, updatedNano)
	return &rv, nil
}

func scanReviews(rows *sql.Rows) ([]*domain.Review, error) {
	var out []*domain.Review
	for rows.Next() {
		rv, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func encodeCursor(rv *domain.Review) string {
	raw := fmt.Sprintf("%d|%s", rv.CreatedAt.UnixNano(), rv.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string, sorted []*domain.Review) int {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return 0
	}
	id := parts[1]
	for i, rv := range sorted {
		if rv.ID == id {
			return i + 1
		}
	}
	return 0
}
