package repository

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/shubhambakre/omnimart-ratings-reviews/internal/domain"
)

// EncodeCursor encodes a review's position as a stable opaque cursor string.
// Format: base64(createdAt_nanos|id) — stable under concurrent writes because
// createdAt is immutable after creation and IDs are unique.
func EncodeCursor(rv *domain.Review) string {
	raw := fmt.Sprintf("%d|%s", rv.CreatedAt.UnixNano(), rv.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor returns the index of the first item after cursor in sorted, or
// 0 if the cursor is absent, malformed, or no longer present in the slice.
func DecodeCursor(cursor string, sorted []*domain.Review) int {
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
