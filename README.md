# Walmart Ratings & Reviews — POC (Go)

A runnable proof-of-concept for a Walmart-style ratings & reviews platform, with **two physically separate API surfaces**:

| Tier | Port | Audience | Auth | Examples |
|------|------|----------|------|----------|
| Site-facing | `:8080` | Public (shoppers, mobile apps, edge cache) | Customer JWT (stub: `X-Customer-Id`) | List approved reviews, submit a review, vote helpful |
| Non-site-facing | `:8081` | Internal (moderation tools, ingest jobs, ops) | mTLS / SPIFFE (stub: `X-Api-Key`) | Pending queue, approve/reject, bulk ingest, recompute aggregates |

## Why two listeners?

In production these would terminate in different network zones — site-facing behind the Walmart CDN/WAF, internal behind the service mesh with mTLS. Sharing a TCP listener is the kind of architectural shortcut that ends up on a postmortem ("a public route accidentally exposed the moderation queue"). Different ports → different middleware stacks → no accidental crossover. The POC encodes this from day one.

## Design highlights (Principal-engineer rationale)

- **Hexagonal layout**: `domain → repository (interface) → service → transport`. Swap the in-memory repo for Cassandra/MongoDB by writing one new package; no handler changes.
- **Aggregate caching**: `RatingSummary` lives in its own store, recomputed on approval. The `RatingService.Recompute` seam is exactly where you'd plug in a Kafka consumer of `review.approved` events for debounced async aggregation in production.
- **Read/write asymmetry**: site-facing reads only return `APPROVED`; the moderation queue is internal-only. Pending or rejected reviews never leak to the product page.
- **Idempotency**: `POST /v1/products/{id}/reviews` honors `Idempotency-Key` — duplicate submits return the same review id. Critical for retry storms from flaky mobile clients.
- **Cursor pagination** on the hot read path (stable under writes); offset on the small moderation queue.
- **State machine**: `PENDING → APPROVED | REJECTED`. Auto-approval if moderation is clean; otherwise human review via the internal API.
- **Observability**: structured logs (`log/slog`), request IDs propagated through context and reflected in `X-Request-Id`.
- **Graceful shutdown** on `SIGTERM`, sane HTTP timeouts (defending against slowloris and runaway clients).
- **Zero external deps**: stdlib only, using Go 1.22's `net/http` method-prefix patterns. Easy to audit for a POC.

## What's intentionally stubbed (and the upgrade path)

| POC | Production |
|-----|------------|
| In-memory repo | Cassandra (write-heavy, productID partition) for reviews; Redis for hot summaries |
| Inline aggregate recompute | Kafka topic `review.approved` → debounced consumer with windowed aggregation |
| `X-Customer-Id` header | OAuth/JWT issued by shopper identity service, scope-checked |
| `X-Api-Key` header | mTLS + SPIFFE service identity via Istio/Linkerd |
| Word-list moderation | Async call to ML moderation service (profanity, PII, spam) + manual queue |
| No rate limiting | Per-customer + per-IP token-bucket at the edge (Akamai/Envoy) |
| No metrics endpoint | Prometheus `/metrics`, RED-method dashboards, SLO burn alerts |
| No event publishing | Outbox pattern → Kafka for downstream (search index, recommendation, fraud) |

## Running locally

Requires Go 1.22+.

```bash
cd walmart-ratings-reviews
make run        # starts both servers, seeded with sample data
```

You'll see something like:

```
{"level":"INFO","msg":"servers up","site":":8080","internal":":8081"}
```

In another terminal:

```bash
make curl-demo  # runs the smoke-test script
make test       # runs the e2e test suite
```

## API reference

### Site-facing (`:8080`)

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET    | `/healthz` | — | Liveness |
| GET    | `/v1/products/{productId}/reviews?cursor=&limit=` | — | Paginated approved reviews |
| GET    | `/v1/products/{productId}/ratings/summary` | — | Avg + distribution |
| GET    | `/v1/reviews/{reviewId}` | — | Single approved review |
| POST   | `/v1/products/{productId}/reviews` | `X-Customer-Id`, optional `Idempotency-Key` | Submit a review |
| POST   | `/v1/reviews/{reviewId}/helpful` | `X-Customer-Id` | Mark helpful |
| POST   | `/v1/reviews/{reviewId}/unhelpful` | `X-Customer-Id` | Mark unhelpful |

### Non-site-facing (`:8081`, all require `X-Api-Key`)

| Method | Path | Purpose |
|--------|------|---------|
| GET    | `/healthz` | Liveness (no key) |
| GET    | `/internal/v1/reviews/pending?offset=&limit=` | Moderation queue |
| GET    | `/internal/v1/reviews/{reviewId}` | Admin view (any status) |
| PATCH  | `/internal/v1/reviews/{reviewId}/moderation` | Approve/reject |
| DELETE | `/internal/v1/reviews/{reviewId}` | Hard delete |
| POST   | `/internal/v1/reviews/bulk` | Bulk ingest (1..1000) |
| POST   | `/internal/v1/products/{productId}/ratings/recompute` | Force recompute |

## Project layout

```
walmart-ratings-reviews/
├── cmd/server/main.go                       # composition root, two http.Servers
├── internal/
│   ├── config/                              # env-driven config
│   ├── domain/                              # entities + sentinel errors
│   ├── repository/                          # storage interfaces
│   │   └── memory/                          # in-memory implementation
│   ├── moderation/                          # pluggable moderator
│   ├── service/                             # use-case orchestration
│   │   ├── review_service.go
│   │   └── rating_service.go
│   └── transport/http/
│       ├── middleware/                      # request id, logger, recover, auth
│       ├── response/                        # JSON envelope + error mapping
│       ├── sitefacing/                      # public handlers
│       └── nonsitefacing/                   # internal handlers
├── tests/e2e_test.go                        # in-process httptest end-to-end
├── scripts/curl_examples.sh                 # live-server smoke test
├── Makefile
└── go.mod
```

## Configuration

| Env var | Default | Notes |
|---------|---------|-------|
| `SITE_ADDR` | `:8080` | Site-facing listener |
| `INTERNAL_ADDR` | `:8081` | Internal listener |
| `INTERNAL_API_KEY` | `dev-internal-key` | Required header for internal API |
| `SEED` | `true` | Load sample reviews on startup |
