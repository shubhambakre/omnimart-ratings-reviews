package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/domain"
)

type Envelope struct {
	Data  any    `json:"data,omitempty"`
	Error *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data})
}

func Err(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Error: &Error{Code: code, Message: msg}})
}

func FromDomainErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrReviewNotFound):
		Err(w, http.StatusNotFound, "REVIEW_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrInvalidRating),
		errors.Is(err, domain.ErrEmptyBody),
		errors.Is(err, domain.ErrTitleTooLong),
		errors.Is(err, domain.ErrBodyTooLong):
		Err(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
	case errors.Is(err, domain.ErrInvalidStatus):
		Err(w, http.StatusConflict, "INVALID_STATE", err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		Err(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
	default:
		Err(w, http.StatusInternalServerError, "INTERNAL", "internal error")
	}
}
