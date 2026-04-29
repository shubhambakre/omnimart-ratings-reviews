package nonsitefacing

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/service"
	"github.com/shubhambakre/omnimart-ratings-reviews/internal/transport/http/response"
)

type Handler struct {
	Reviews *service.ReviewService
	Ratings *service.RatingService
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /internal/v1/reviews/pending", h.listPending)
	mux.HandleFunc("GET /internal/v1/reviews/{reviewId}", h.getReview)
	mux.HandleFunc("PATCH /internal/v1/reviews/{reviewId}/moderation", h.moderate)
	mux.HandleFunc("DELETE /internal/v1/reviews/{reviewId}", h.delete)
	mux.HandleFunc("POST /internal/v1/reviews/bulk", h.bulk)
	mux.HandleFunc("POST /internal/v1/products/{productId}/ratings/recompute", h.recompute)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok", "tier": "internal"})
	})
}

func (h *Handler) listPending(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, total, err := h.Reviews.ListPendingModeration(r.Context(), offset, limit)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"reviews": items,
		"total":   total,
		"offset":  offset,
	})
}

// Internal getReview returns the review regardless of status (moderators need
// to see PENDING/REJECTED items; the site-facing endpoint hides them).
func (h *Handler) getReview(w http.ResponseWriter, r *http.Request) {
	rv, err := h.Reviews.Get(r.Context(), r.PathValue("reviewId"), true)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, rv)
}

type moderateReq struct {
	Approve bool   `json:"approve"`
	Notes   string `json:"notes,omitempty"`
}

func (h *Handler) moderate(w http.ResponseWriter, r *http.Request) {
	var req moderateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	rv, err := h.Reviews.Moderate(r.Context(), r.PathValue("reviewId"), service.ModerationDecision{
		Approve: req.Approve,
		Notes:   req.Notes,
	})
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, rv)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.Reviews.Delete(r.Context(), r.PathValue("reviewId")); err != nil {
		response.FromDomainErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bulkItem struct {
	ProductID        string `json:"productId"`
	CustomerID       string `json:"customerId"`
	OrderID          string `json:"orderId,omitempty"`
	Rating           int    `json:"rating"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	VerifiedPurchase bool   `json:"verifiedPurchase"`
}

func (h *Handler) bulk(w http.ResponseWriter, r *http.Request) {
	var items []bulkItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		response.Err(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if len(items) == 0 || len(items) > 1000 {
		response.Err(w, http.StatusBadRequest, "VALIDATION_FAILED", "batch size must be 1..1000")
		return
	}
	inputs := make([]service.SubmitReviewInput, 0, len(items))
	for _, it := range items {
		inputs = append(inputs, service.SubmitReviewInput{
			ProductID:        it.ProductID,
			CustomerID:       it.CustomerID,
			OrderID:          it.OrderID,
			Rating:           it.Rating,
			Title:            it.Title,
			Body:             it.Body,
			VerifiedPurchase: it.VerifiedPurchase,
		})
	}
	res := h.Reviews.BulkIngest(r.Context(), inputs)
	response.JSON(w, http.StatusAccepted, res)
}

func (h *Handler) recompute(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productId")
	if err := h.Ratings.Recompute(r.Context(), productID); err != nil {
		response.FromDomainErr(w, err)
		return
	}
	s, err := h.Ratings.Get(r.Context(), productID)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}
