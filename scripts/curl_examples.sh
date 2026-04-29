#!/usr/bin/env bash
# End-to-end smoke test against a locally-running server.
# Run `make run` in another terminal first.
set -euo pipefail

SITE=${SITE:-http://localhost:8080}
INT=${INT:-http://localhost:8081}
KEY=${INTERNAL_API_KEY:-dev-internal-key}

echo "== site: list seeded reviews for PROD-1 =="
curl -s "$SITE/v1/products/PROD-1/reviews" | sed 's/.*/&/'
echo

echo "== site: rating summary for PROD-1 =="
curl -s "$SITE/v1/products/PROD-1/ratings/summary"
echo

echo "== site: submit a clean review (auto-approved) =="
curl -s -X POST "$SITE/v1/products/PROD-9/reviews" \
  -H 'Content-Type: application/json' \
  -H 'X-Customer-Id: C-99' \
  -H 'Idempotency-Key: demo-1' \
  -d '{"rating":5,"title":"Loved it","body":"Works as advertised, recommend.","verifiedPurchase":true}'
echo

echo "== site: submit a flagged review (goes to PENDING) =="
RESP=$(curl -s -X POST "$SITE/v1/products/PROD-9/reviews" \
  -H 'Content-Type: application/json' \
  -H 'X-Customer-Id: C-100' \
  -d '{"rating":1,"title":"scam!!","body":"this is a scam, do not buy"}')
echo "$RESP"
PENDING_ID=$(echo "$RESP" | sed -E 's/.*"id":"([^"]+)".*/\1/')
echo "pending id: $PENDING_ID"
echo

echo "== internal: pending moderation queue (requires API key) =="
curl -s -H "X-Api-Key: $KEY" "$INT/internal/v1/reviews/pending"
echo

echo "== internal: approve the pending review =="
curl -s -X PATCH "$INT/internal/v1/reviews/$PENDING_ID/moderation" \
  -H 'Content-Type: application/json' \
  -H "X-Api-Key: $KEY" \
  -d '{"approve":true,"notes":"manual override"}'
echo

echo "== site: list reviews for PROD-9 (should now include the approved one) =="
curl -s "$SITE/v1/products/PROD-9/reviews"
echo

echo "== internal: bulk ingest =="
curl -s -X POST "$INT/internal/v1/reviews/bulk" \
  -H 'Content-Type: application/json' \
  -H "X-Api-Key: $KEY" \
  -d '[{"productId":"PROD-2","customerId":"C-200","rating":4,"title":"Nice","body":"Pretty good overall"},{"productId":"PROD-2","customerId":"C-201","rating":5,"title":"Top","body":"Excellent product, would buy again"}]'
echo

echo "== internal: recompute aggregate for PROD-2 =="
curl -s -X POST -H "X-Api-Key: $KEY" "$INT/internal/v1/products/PROD-2/ratings/recompute"
echo
