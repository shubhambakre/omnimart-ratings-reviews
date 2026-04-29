package sitefacing

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/walmart/ratings-reviews/internal/service"
	"github.com/walmart/ratings-reviews/internal/transport/http/middleware"
	"github.com/walmart/ratings-reviews/internal/transport/http/response"
)

type Handler struct {
	Reviews *service.ReviewService
	Ratings *service.RatingService
}

func (h *Handler) Mount(mux *http.ServeMux) {
	// Public reads — open to anonymous traffic.
	mux.Handle("GET /v1/products/{productId}/reviews",
		middleware.Chain(http.HandlerFunc(h.listReviews), middleware.CustomerAuth(false)))
	mux.Handle("GET /v1/products/{productId}/ratings/summary",
		middleware.Chain(http.HandlerFunc(h.summary), middleware.CustomerAuth(false)))
	mux.Handle("GET /v1/reviews/{reviewId}",
		middleware.Chain(http.HandlerFunc(h.getReview), middleware.CustomerAuth(false)))

	// Authenticated writes — require X-Customer-Id.
	mux.Handle("POST /v1/products/{productId}/reviews",
		middleware.Chain(http.HandlerFunc(h.submit), middleware.CustomerAuth(true)))
	mux.Handle("POST /v1/reviews/{reviewId}/helpful",
		middleware.Chain(http.HandlerFunc(h.voteHelpful), middleware.CustomerAuth(true)))
	mux.Handle("POST /v1/reviews/{reviewId}/unhelpful",
		middleware.Chain(http.HandlerFunc(h.voteUnhelpful), middleware.CustomerAuth(true)))

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok", "tier": "site-facing"})
	})
}

func (h *Handler) listReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productId")
	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, next, err := h.Reviews.ListPublic(r.Context(), productID, cursor, limit)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	response.JSON(w, http.StatusOK, map[string]any{
		"reviews":    items,
		"nextCursor": next,
	})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productId")
	s, err := h.Ratings.Get(r.Context(), productID)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	response.JSON(w, http.StatusOK, s)
}

func (h *Handler) getReview(w http.ResponseWriter, r *http.Request) {
	rv, err := h.Reviews.Get(r.Context(), r.PathValue("reviewId"), false)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, rv)
}

type submitReq struct {
	Rating           int    `json:"rating"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	OrderID          string `json:"orderId,omitempty"`
	VerifiedPurchase bool   `json:"verifiedPurchase"`
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	var req submitReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	cid, _ := r.Context().Value(middleware.CtxCustomer).(string)
	rv, err := h.Reviews.Submit(r.Context(), service.SubmitReviewInput{
		ProductID:        r.PathValue("productId"),
		CustomerID:       cid,
		OrderID:          req.OrderID,
		Rating:           req.Rating,
		Title:            req.Title,
		Body:             req.Body,
		VerifiedPurchase: req.VerifiedPurchase,
		IdempotencyKey:   r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, rv)
}

func (h *Handler) voteHelpful(w http.ResponseWriter, r *http.Request) {
	rv, err := h.Reviews.Vote(r.Context(), r.PathValue("reviewId"), true)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, rv)
}

func (h *Handler) voteUnhelpful(w http.ResponseWriter, r *http.Request) {
	rv, err := h.Reviews.Vote(r.Context(), r.PathValue("reviewId"), false)
	if err != nil {
		response.FromDomainErr(w, err)
		return
	}
	response.JSON(w, http.StatusOK, rv)
}
