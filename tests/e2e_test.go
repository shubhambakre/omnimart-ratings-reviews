package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/walmart/ratings-reviews/internal/domain"
	"github.com/walmart/ratings-reviews/internal/moderation"
	"github.com/walmart/ratings-reviews/internal/repository/memory"
	"github.com/walmart/ratings-reviews/internal/service"
	"github.com/walmart/ratings-reviews/internal/transport/http/middleware"
	"github.com/walmart/ratings-reviews/internal/transport/http/nonsitefacing"
	"github.com/walmart/ratings-reviews/internal/transport/http/sitefacing"
)

const internalKey = "test-key"

type harness struct {
	site, internal *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reviewRepo := memory.NewReviewRepo()
	ratingRepo := memory.NewRatingRepo()
	mod := moderation.NewSimpleModerator()
	ratingSvc := service.NewRatingService(reviewRepo, ratingRepo, log)
	reviewSvc := service.NewReviewService(reviewRepo, ratingRepo, mod, ratingSvc, log)

	siteMux := http.NewServeMux()
	(&sitefacing.Handler{Reviews: reviewSvc, Ratings: ratingSvc}).Mount(siteMux)
	siteH := middleware.Chain(siteMux, middleware.RequestID, middleware.Logger(log), middleware.Recover(log))

	intMux := http.NewServeMux()
	(&nonsitefacing.Handler{Reviews: reviewSvc, Ratings: ratingSvc}).Mount(intMux)
	intH := middleware.Chain(intMux, middleware.RequestID, middleware.Logger(log), middleware.Recover(log), middleware.APIKeyAuth(internalKey))

	return &harness{
		site:     httptest.NewServer(siteH),
		internal: httptest.NewServer(intH),
	}
}

func (h *harness) close() {
	h.site.Close()
	h.internal.Close()
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func do(t *testing.T, method, url string, body any, headers map[string]string) (*http.Response, envelope) {
	t.Helper()
	var br io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		br = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, br)
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp, env
}

func TestSiteFacing_SubmitListSummary(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// Submit a clean review — auto-approves.
	resp, env := do(t, http.MethodPost, h.site.URL+"/v1/products/PROD-X/reviews",
		map[string]any{"rating": 5, "title": "Awesome", "body": "Truly loved this product."},
		map[string]string{"X-Customer-Id": "C-1", "Idempotency-Key": "key-1"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit: got %d, body=%s", resp.StatusCode, string(env.Data))
	}
	var rv domain.Review
	_ = json.Unmarshal(env.Data, &rv)
	if rv.Status != domain.StatusApproved {
		t.Fatalf("expected APPROVED, got %s", rv.Status)
	}

	// Idempotent retry returns same id.
	resp2, env2 := do(t, http.MethodPost, h.site.URL+"/v1/products/PROD-X/reviews",
		map[string]any{"rating": 5, "title": "Awesome", "body": "Truly loved this product."},
		map[string]string{"X-Customer-Id": "C-1", "Idempotency-Key": "key-1"})
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("retry: %d", resp2.StatusCode)
	}
	var rv2 domain.Review
	_ = json.Unmarshal(env2.Data, &rv2)
	if rv2.ID != rv.ID {
		t.Fatalf("idempotency broken: %s != %s", rv2.ID, rv.ID)
	}

	// Listing.
	resp3, env3 := do(t, http.MethodGet, h.site.URL+"/v1/products/PROD-X/reviews", nil, nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", resp3.StatusCode)
	}
	var listed struct {
		Reviews    []domain.Review `json:"reviews"`
		NextCursor string          `json:"nextCursor"`
	}
	_ = json.Unmarshal(env3.Data, &listed)
	if len(listed.Reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(listed.Reviews))
	}

	// Summary.
	resp4, env4 := do(t, http.MethodGet, h.site.URL+"/v1/products/PROD-X/ratings/summary", nil, nil)
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("summary: %d", resp4.StatusCode)
	}
	var sum domain.RatingSummary
	_ = json.Unmarshal(env4.Data, &sum)
	if sum.TotalReviews != 1 || sum.AverageRating != 5.0 {
		t.Fatalf("bad summary: %+v", sum)
	}
}

func TestSiteFacing_AuthRequired(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	resp, env := do(t, http.MethodPost, h.site.URL+"/v1/products/P/reviews",
		map[string]any{"rating": 5, "title": "x", "body": "y"}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED error code, got %+v", env.Error)
	}
}

func TestModerationFlow(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// Submit a review with a banned word — goes to PENDING.
	resp, env := do(t, http.MethodPost, h.site.URL+"/v1/products/PROD-Y/reviews",
		map[string]any{"rating": 1, "title": "scam alert", "body": "this is a scam product"},
		map[string]string{"X-Customer-Id": "C-2"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit: %d", resp.StatusCode)
	}
	var rv domain.Review
	_ = json.Unmarshal(env.Data, &rv)
	if rv.Status != domain.StatusPending {
		t.Fatalf("expected PENDING, got %s", rv.Status)
	}

	// Pending review must NOT show up on the public list.
	resp2, env2 := do(t, http.MethodGet, h.site.URL+"/v1/products/PROD-Y/reviews", nil, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", resp2.StatusCode)
	}
	var listed struct {
		Reviews []domain.Review `json:"reviews"`
	}
	_ = json.Unmarshal(env2.Data, &listed)
	if len(listed.Reviews) != 0 {
		t.Fatalf("pending review leaked to public, got %d", len(listed.Reviews))
	}

	// Internal API requires the key.
	respNoKey, _ := do(t, http.MethodGet, h.internal.URL+"/internal/v1/reviews/pending", nil, nil)
	if respNoKey.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", respNoKey.StatusCode)
	}

	// With the key, moderator approves.
	authH := map[string]string{"X-Api-Key": internalKey}
	resp3, _ := do(t, http.MethodPatch, h.internal.URL+"/internal/v1/reviews/"+rv.ID+"/moderation",
		map[string]any{"approve": true, "notes": "false positive"}, authH)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("moderate: %d", resp3.StatusCode)
	}

	// Now visible publicly.
	resp4, env4 := do(t, http.MethodGet, h.site.URL+"/v1/products/PROD-Y/reviews", nil, nil)
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("list2: %d", resp4.StatusCode)
	}
	var listed2 struct {
		Reviews []domain.Review `json:"reviews"`
	}
	_ = json.Unmarshal(env4.Data, &listed2)
	if len(listed2.Reviews) != 1 {
		t.Fatalf("expected 1 after approval, got %d", len(listed2.Reviews))
	}
}

func TestBulkAndRecompute(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	authH := map[string]string{"X-Api-Key": internalKey}
	body := []map[string]any{
		{"productId": "BULK-1", "customerId": "C-A", "rating": 5, "title": "ok", "body": "great item"},
		{"productId": "BULK-1", "customerId": "C-B", "rating": 3, "title": "ok", "body": "average item"},
		{"productId": "BULK-1", "customerId": "C-C", "rating": 4, "title": "ok", "body": "decent item"},
	}
	resp, _ := do(t, http.MethodPost, h.internal.URL+"/internal/v1/reviews/bulk", body, authH)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk: %d", resp.StatusCode)
	}

	resp2, env2 := do(t, http.MethodPost, h.internal.URL+"/internal/v1/products/BULK-1/ratings/recompute", nil, authH)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("recompute: %d", resp2.StatusCode)
	}
	var sum domain.RatingSummary
	_ = json.Unmarshal(env2.Data, &sum)
	if sum.TotalReviews != 3 {
		t.Fatalf("expected 3 reviews, got %d", sum.TotalReviews)
	}
	if sum.AverageRating != 4.0 {
		t.Fatalf("expected avg 4.0, got %v", sum.AverageRating)
	}
}

func TestPagination(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	hdr := map[string]string{"X-Customer-Id": "C-PAG"}
	for i := 0; i < 5; i++ {
		body := map[string]any{"rating": 5, "title": "t", "body": "good item"}
		resp, _ := do(t, http.MethodPost, h.site.URL+"/v1/products/PG/reviews", body, hdr)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("seed %d: %d", i, resp.StatusCode)
		}
	}
	// First page (limit=2)
	resp, env := do(t, http.MethodGet, h.site.URL+"/v1/products/PG/reviews?limit=2", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page1: %d", resp.StatusCode)
	}
	var p1 struct {
		Reviews    []domain.Review `json:"reviews"`
		NextCursor string          `json:"nextCursor"`
	}
	_ = json.Unmarshal(env.Data, &p1)
	if len(p1.Reviews) != 2 || p1.NextCursor == "" {
		t.Fatalf("page1 unexpected: %+v", p1)
	}
	// Second page
	resp2, env2 := do(t, http.MethodGet, h.site.URL+"/v1/products/PG/reviews?limit=2&cursor="+p1.NextCursor, nil, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("page2: %d", resp2.StatusCode)
	}
	var p2 struct {
		Reviews []domain.Review `json:"reviews"`
	}
	_ = json.Unmarshal(env2.Data, &p2)
	if len(p2.Reviews) != 2 {
		t.Fatalf("page2 expected 2, got %d", len(p2.Reviews))
	}
	if p2.Reviews[0].ID == p1.Reviews[0].ID || p2.Reviews[0].ID == p1.Reviews[1].ID {
		t.Fatalf("page2 overlaps page1")
	}
}
