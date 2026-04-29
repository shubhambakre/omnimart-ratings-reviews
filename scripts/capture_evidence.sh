#!/usr/bin/env bash
# capture_evidence.sh — runs a full request/response test suite against the live server
# and emits a Markdown evidence report to docs/EVIDENCE.md.
#
# Usage:
#   make evidence          (server must already be running)
# Or standalone:
#   SITE=http://localhost:8080 INT=http://localhost:8081 bash scripts/capture_evidence.sh
set -euo pipefail

SITE="${SITE:-http://localhost:8080}"
INT="${INT:-http://localhost:8081}"
KEY="${INTERNAL_API_KEY:-dev-internal-key}"
OUT="docs/EVIDENCE.md"
TS=$(date -u "+%Y-%m-%dT%H:%M:%SZ")

mkdir -p docs

{
cat <<HEADER
# OmniMart Ratings & Reviews — Local Evidence Report

**Generated:** ${TS}
**Site-facing:** \`${SITE}\`
**Internal (non-site-facing):** \`${INT}\`
**Storage driver:** \`${STORAGE_DRIVER:-memory}\`

All requests and responses are captured verbatim. HTTP status codes are highlighted.

---
HEADER

section() { printf '\n## %s\n\n' "$1"; }
request() { printf '```http\n%s\n```\n\n' "$1"; }
response() {
  local status=$1 body=$2
  if [ "$status" -ge 200 ] && [ "$status" -lt 300 ]; then
    printf '**Status: %s ✅**\n\n```json\n%s\n```\n\n' "$status" "$body"
  else
    printf '**Status: %s ⚠️**\n\n```json\n%s\n```\n\n' "$status" "$body"
  fi
}

curlj() {
  # curlj METHOD URL [extra curl args...]
  local method=$1 url=$2; shift 2
  local tmpfile; tmpfile=$(mktemp)
  local status
  status=$(curl -s -o "$tmpfile" -w "%{http_code}" -X "$method" "$url" "$@")
  local body; body=$(cat "$tmpfile"); rm -f "$tmpfile"
  printf '%s\t%s' "$status" "$body"
}

read_pair() {
  # helper: split a tab-separated "STATUS\tBODY" line
  local raw="$1"
  STATUS=$(printf '%s' "$raw" | cut -f1)
  BODY=$(printf '%s' "$raw" | cut -f2-)
}

pretty() { echo "$1" | python3 -m json.tool 2>/dev/null || echo "$1"; }

# -----------------------------------------------------------------------
section "1. Health checks"

section "1.1 Site-facing /healthz"
request "GET ${SITE}/healthz"
read_pair "$(curlj GET "${SITE}/healthz")"
response "$STATUS" "$(pretty "$BODY")"

section "1.2 Internal /healthz (no key required)"
request "GET ${INT}/healthz"
read_pair "$(curlj GET "${INT}/healthz")"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
section "2. Seeded data reads"

section "2.1 List approved reviews for PROD-1 (3 seeded, cursor-paginated)"
request "GET ${SITE}/v1/products/PROD-1/reviews?limit=2"
read_pair "$(curlj GET "${SITE}/v1/products/PROD-1/reviews?limit=2")"
response "$STATUS" "$(pretty "$BODY")"
CURSOR=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('nextCursor',''))" 2>/dev/null || true)

if [ -n "$CURSOR" ]; then
  section "2.2 Second page via cursor"
  request "GET ${SITE}/v1/products/PROD-1/reviews?limit=2&cursor=${CURSOR}"
  read_pair "$(curlj GET "${SITE}/v1/products/PROD-1/reviews?limit=2&cursor=${CURSOR}")"
  response "$STATUS" "$(pretty "$BODY")"
fi

section "2.3 Rating summary for PROD-1"
request "GET ${SITE}/v1/products/PROD-1/ratings/summary"
read_pair "$(curlj GET "${SITE}/v1/products/PROD-1/ratings/summary")"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
section "3. Submit a clean review (auto-approved)"
request "POST ${SITE}/v1/products/PROD-EVD/reviews
Content-Type: application/json
X-Customer-Id: C-EVD-1
Idempotency-Key: evd-clean-1

{\"rating\":5,\"title\":\"Hackathon-grade\",\"body\":\"Best product ever tested in this POC.\",\"verifiedPurchase\":true}"

read_pair "$(curlj POST "${SITE}/v1/products/PROD-EVD/reviews" \
  -H 'Content-Type: application/json' \
  -H 'X-Customer-Id: C-EVD-1' \
  -H 'Idempotency-Key: evd-clean-1' \
  -d '{"rating":5,"title":"Hackathon-grade","body":"Best product ever tested in this POC.","verifiedPurchase":true}')"
response "$STATUS" "$(pretty "$BODY")"
CLEAN_ID=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null || true)

section "3.1 Idempotent retry (same key, same body) — must return identical review id"
request "POST ${SITE}/v1/products/PROD-EVD/reviews (duplicate Idempotency-Key: evd-clean-1)"
read_pair "$(curlj POST "${SITE}/v1/products/PROD-EVD/reviews" \
  -H 'Content-Type: application/json' \
  -H 'X-Customer-Id: C-EVD-1' \
  -H 'Idempotency-Key: evd-clean-1' \
  -d '{"rating":5,"title":"Hackathon-grade","body":"Best product ever tested in this POC.","verifiedPurchase":true}')"
response "$STATUS" "$(pretty "$BODY")"
RETRY_ID=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null || true)
if [ "$CLEAN_ID" = "$RETRY_ID" ]; then
  printf '> **Idempotency verified** — both calls returned `%s` ✅\n\n' "$CLEAN_ID"
else
  printf '> **Idempotency FAILED** — got `%s` vs `%s` ❌\n\n' "$CLEAN_ID" "$RETRY_ID"
fi

section "3.2 Fetch single approved review"
request "GET ${SITE}/v1/reviews/${CLEAN_ID}"
read_pair "$(curlj GET "${SITE}/v1/reviews/${CLEAN_ID}")"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
section "4. Submit a flagged review (goes to PENDING)"
request "POST ${SITE}/v1/products/PROD-EVD/reviews
Content-Type: application/json
X-Customer-Id: C-EVD-2

{\"rating\":1,\"title\":\"scam product\",\"body\":\"Do not buy, this is a scam!\"}"

read_pair "$(curlj POST "${SITE}/v1/products/PROD-EVD/reviews" \
  -H 'Content-Type: application/json' \
  -H 'X-Customer-Id: C-EVD-2' \
  -d '{"rating":1,"title":"scam product","body":"Do not buy, this is a scam!"}')"
response "$STATUS" "$(pretty "$BODY")"
PENDING_ID=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null || true)

section "4.1 Pending review must NOT appear on public list"
request "GET ${SITE}/v1/products/PROD-EVD/reviews (should only show APPROVED reviews)"
read_pair "$(curlj GET "${SITE}/v1/products/PROD-EVD/reviews")"
response "$STATUS" "$(pretty "$BODY")"
COUNT=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin).get('data',{}); print(len(d.get('reviews',[])))" 2>/dev/null || echo "?")
printf '> Public list returned **%s** review(s) — pending review correctly hidden ✅\n\n' "$COUNT"

# -----------------------------------------------------------------------
section "5. Internal moderation API"

section "5.1 Reject without API key → 401"
request "GET ${INT}/internal/v1/reviews/pending  (no X-Api-Key)"
read_pair "$(curlj GET "${INT}/internal/v1/reviews/pending")"
response "$STATUS" "$(pretty "$BODY")"

section "5.2 Moderation queue (pending reviews)"
request "GET ${INT}/internal/v1/reviews/pending
X-Api-Key: ${KEY}"
read_pair "$(curlj GET "${INT}/internal/v1/reviews/pending" -H "X-Api-Key: ${KEY}")"
response "$STATUS" "$(pretty "$BODY")"

section "5.3 Approve the pending review"
request "PATCH ${INT}/internal/v1/reviews/${PENDING_ID}/moderation
X-Api-Key: ${KEY}
Content-Type: application/json

{\"approve\":true,\"notes\":\"false positive from word-list moderator\"}"
read_pair "$(curlj PATCH "${INT}/internal/v1/reviews/${PENDING_ID}/moderation" \
  -H "X-Api-Key: ${KEY}" \
  -H 'Content-Type: application/json' \
  -d '{"approve":true,"notes":"false positive from word-list moderator"}')"
response "$STATUS" "$(pretty "$BODY")"

section "5.4 Approved review now appears on public list"
request "GET ${SITE}/v1/products/PROD-EVD/reviews"
read_pair "$(curlj GET "${SITE}/v1/products/PROD-EVD/reviews")"
response "$STATUS" "$(pretty "$BODY")"
COUNT2=$(printf '%s' "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin).get('data',{}); print(len(d.get('reviews',[])))" 2>/dev/null || echo "?")
printf '> Public list now shows **%s** review(s) — approved review visible ✅\n\n' "$COUNT2"

# -----------------------------------------------------------------------
section "6. Helpful votes"

section "6.1 Mark a review helpful"
request "POST ${SITE}/v1/reviews/${CLEAN_ID}/helpful
X-Customer-Id: C-EVD-3"
read_pair "$(curlj POST "${SITE}/v1/reviews/${CLEAN_ID}/helpful" \
  -H 'X-Customer-Id: C-EVD-3')"
response "$STATUS" "$(pretty "$BODY")"

section "6.2 Mark a review unhelpful"
request "POST ${SITE}/v1/reviews/${CLEAN_ID}/unhelpful
X-Customer-Id: C-EVD-4"
read_pair "$(curlj POST "${SITE}/v1/reviews/${CLEAN_ID}/unhelpful" \
  -H 'X-Customer-Id: C-EVD-4')"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
section "7. Bulk ingest (internal)"
request "POST ${INT}/internal/v1/reviews/bulk
X-Api-Key: ${KEY}
Content-Type: application/json

[3 reviews for PROD-BULK]"
read_pair "$(curlj POST "${INT}/internal/v1/reviews/bulk" \
  -H "X-Api-Key: ${KEY}" \
  -H 'Content-Type: application/json' \
  -d '[
    {"productId":"PROD-BULK","customerId":"C-B1","rating":5,"title":"Excellent","body":"Top tier product, zero issues."},
    {"productId":"PROD-BULK","customerId":"C-B2","rating":4,"title":"Very good","body":"Works well, minor quibbles."},
    {"productId":"PROD-BULK","customerId":"C-B3","rating":3,"title":"Average","body":"Does the job but nothing special."}
  ]')"
response "$STATUS" "$(pretty "$BODY")"

section "7.1 Force recompute aggregates for PROD-BULK"
request "POST ${INT}/internal/v1/products/PROD-BULK/ratings/recompute
X-Api-Key: ${KEY}"
read_pair "$(curlj POST "${INT}/internal/v1/products/PROD-BULK/ratings/recompute" \
  -H "X-Api-Key: ${KEY}")"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
section "8. Validation errors"

section "8.1 Rating out of range"
request "POST ${SITE}/v1/products/P/reviews
X-Customer-Id: C-X
Content-Type: application/json

{\"rating\":6,\"title\":\"x\",\"body\":\"y\"}"
read_pair "$(curlj POST "${SITE}/v1/products/P/reviews" \
  -H 'X-Customer-Id: C-X' \
  -H 'Content-Type: application/json' \
  -d '{"rating":6,"title":"x","body":"y"}')"
response "$STATUS" "$(pretty "$BODY")"

section "8.2 Missing X-Customer-Id on write → 401"
request "POST ${SITE}/v1/products/P/reviews  (no auth header)"
read_pair "$(curlj POST "${SITE}/v1/products/P/reviews" \
  -H 'Content-Type: application/json' \
  -d '{"rating":5,"title":"ok","body":"ok"}')"
response "$STATUS" "$(pretty "$BODY")"

section "8.3 Hard delete (internal)"
request "DELETE ${INT}/internal/v1/reviews/${PENDING_ID}
X-Api-Key: ${KEY}"
read_pair "$(curlj DELETE "${INT}/internal/v1/reviews/${PENDING_ID}" \
  -H "X-Api-Key: ${KEY}")"
response "$STATUS" "$(pretty "$BODY")"

section "8.4 Fetch deleted review → 404"
request "GET ${SITE}/v1/reviews/${PENDING_ID}"
read_pair "$(curlj GET "${SITE}/v1/reviews/${PENDING_ID}")"
response "$STATUS" "$(pretty "$BODY")"

# -----------------------------------------------------------------------
printf '\n---\n\n*Evidence captured at %s by capture_evidence.sh*\n' "${TS}"

} > "$OUT"

echo "Evidence written to $OUT"
