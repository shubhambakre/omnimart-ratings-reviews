# OmniMart Ratings & Reviews — Local Evidence Report

**Generated:** 2026-04-29T05:48:56Z
**Site-facing:** `http://localhost:8080`
**Internal (non-site-facing):** `http://localhost:8081`
**Storage driver:** `sqlite`

All requests and responses are captured verbatim. HTTP status codes are highlighted.

---

## 1. Health checks


## 1.1 Site-facing /healthz

```http
GET http://localhost:8080/healthz
```

**Status: 200 ✅**

```json
{
    "data": {
        "status": "ok",
        "tier": "site-facing"
    }
}
```


## 1.2 Internal /healthz (no key required)

```http
GET http://localhost:8081/healthz
```

**Status: 200 ✅**

```json
{
    "data": {
        "status": "ok",
        "tier": "internal"
    }
}
```


## 2. Seeded data reads


## 2.1 List approved reviews for PROD-1 (3 seeded, cursor-paginated)

```http
GET http://localhost:8080/v1/products/PROD-1/reviews?limit=2
```

**Status: 200 ✅**

```json
{
    "data": {
        "nextCursor": "MTc3NzQ0MTYwNzIzMDU4MjAwMHxydl8wNzQ0M2I2OTA3YjdiMDJjOWM1MjI2MmQ",
        "reviews": [
            {
                "id": "rv_fb19f6013b0f560b8e56b63e",
                "productId": "PROD-1",
                "customerId": "C-3",
                "rating": 2,
                "title": "Meh",
                "body": "Smaller than expected, but works fine.",
                "verifiedPurchase": false,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "createdAt": "2026-04-28T22:46:47.230742-07:00",
                "updatedAt": "2026-04-28T22:46:47.230742-07:00"
            },
            {
                "id": "rv_07443b6907b7b02c9c52262d",
                "productId": "PROD-1",
                "customerId": "C-2",
                "rating": 4,
                "title": "Solid",
                "body": "Good quality for the price.",
                "verifiedPurchase": true,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "createdAt": "2026-04-28T22:46:47.230582-07:00",
                "updatedAt": "2026-04-28T22:46:47.230582-07:00"
            }
        ]
    }
}
```


## 2.2 Second page via cursor

```http
GET http://localhost:8080/v1/products/PROD-1/reviews?limit=2&cursor=MTc3NzQ0MTYwNzIzMDU4MjAwMHxydl8wNzQ0M2I2OTA3YjdiMDJjOWM1MjI2MmQ
```

**Status: 200 ✅**

```json
{
    "data": {
        "nextCursor": "",
        "reviews": [
            {
                "id": "rv_49bfb41de9a70d76880a4686",
                "productId": "PROD-1",
                "customerId": "C-1",
                "rating": 5,
                "title": "Great",
                "body": "Loved the product, holds up well.",
                "verifiedPurchase": true,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "createdAt": "2026-04-28T22:46:47.230301-07:00",
                "updatedAt": "2026-04-28T22:46:47.230301-07:00"
            }
        ]
    }
}
```


## 2.3 Rating summary for PROD-1

```http
GET http://localhost:8080/v1/products/PROD-1/ratings/summary
```

**Status: 200 ✅**

```json
{
    "data": {
        "productId": "PROD-1",
        "averageRating": 3.67,
        "totalReviews": 3,
        "distribution": {
            "1": 0,
            "2": 1,
            "3": 0,
            "4": 1,
            "5": 1
        },
        "updatedAt": "2026-04-28T22:46:47.230844-07:00"
    }
}
```


## 3. Submit a clean review (auto-approved)

```http
POST http://localhost:8080/v1/products/PROD-EVD/reviews
Content-Type: application/json
X-Customer-Id: C-EVD-1
Idempotency-Key: evd-clean-1

{"rating":5,"title":"Hackathon-grade","body":"Best product ever tested in this POC.","verifiedPurchase":true}
```

**Status: 201 ✅**

```json
{
    "data": {
        "id": "rv_975682208bf3b32e54b43f5f",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-1",
        "rating": 5,
        "title": "Hackathon-grade",
        "body": "Best product ever tested in this POC.",
        "verifiedPurchase": true,
        "status": "APPROVED",
        "helpfulCount": 0,
        "unhelpfulCount": 0,
        "createdAt": "2026-04-28T22:48:57.066506-07:00",
        "updatedAt": "2026-04-28T22:48:57.066506-07:00"
    }
}
```


## 3.1 Idempotent retry (same key, same body) — must return identical review id

```http
POST http://localhost:8080/v1/products/PROD-EVD/reviews (duplicate Idempotency-Key: evd-clean-1)
```

**Status: 201 ✅**

```json
{
    "data": {
        "id": "rv_975682208bf3b32e54b43f5f",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-1",
        "rating": 5,
        "title": "Hackathon-grade",
        "body": "Best product ever tested in this POC.",
        "verifiedPurchase": true,
        "status": "APPROVED",
        "helpfulCount": 0,
        "unhelpfulCount": 0,
        "createdAt": "2026-04-28T22:48:57.066506-07:00",
        "updatedAt": "2026-04-28T22:48:57.066506-07:00"
    }
}
```

> **Idempotency verified** — both calls returned `rv_975682208bf3b32e54b43f5f` ✅


## 3.2 Fetch single approved review

```http
GET http://localhost:8080/v1/reviews/rv_975682208bf3b32e54b43f5f
```

**Status: 200 ✅**

```json
{
    "data": {
        "id": "rv_975682208bf3b32e54b43f5f",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-1",
        "rating": 5,
        "title": "Hackathon-grade",
        "body": "Best product ever tested in this POC.",
        "verifiedPurchase": true,
        "status": "APPROVED",
        "helpfulCount": 0,
        "unhelpfulCount": 0,
        "createdAt": "2026-04-28T22:48:57.066506-07:00",
        "updatedAt": "2026-04-28T22:48:57.066506-07:00"
    }
}
```


## 4. Submit a flagged review (goes to PENDING)

```http
POST http://localhost:8080/v1/products/PROD-EVD/reviews
Content-Type: application/json
X-Customer-Id: C-EVD-2

{"rating":1,"title":"scam product","body":"Do not buy, this is a scam!"}
```

**Status: 201 ✅**

```json
{
    "data": {
        "id": "rv_3af9dbf37acaff0b282cadc4",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-2",
        "rating": 1,
        "title": "scam product",
        "body": "Do not buy, this is a scam!",
        "verifiedPurchase": false,
        "status": "PENDING",
        "helpfulCount": 0,
        "unhelpfulCount": 0,
        "moderationNotes": "contains banned term: scam",
        "createdAt": "2026-04-28T22:48:57.276683-07:00",
        "updatedAt": "2026-04-28T22:48:57.276683-07:00"
    }
}
```


## 4.1 Pending review must NOT appear on public list

```http
GET http://localhost:8080/v1/products/PROD-EVD/reviews (should only show APPROVED reviews)
```

**Status: 200 ✅**

```json
{
    "data": {
        "nextCursor": "",
        "reviews": [
            {
                "id": "rv_975682208bf3b32e54b43f5f",
                "productId": "PROD-EVD",
                "customerId": "C-EVD-1",
                "rating": 5,
                "title": "Hackathon-grade",
                "body": "Best product ever tested in this POC.",
                "verifiedPurchase": true,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "createdAt": "2026-04-28T22:48:57.066506-07:00",
                "updatedAt": "2026-04-28T22:48:57.066506-07:00"
            }
        ]
    }
}
```

> Public list returned **1** review(s) — pending review correctly hidden ✅


## 5. Internal moderation API


## 5.1 Reject without API key → 401

```http
GET http://localhost:8081/internal/v1/reviews/pending  (no X-Api-Key)
```

**Status: 401 ⚠️**

```json
{
    "error": {
        "code": "UNAUTHORIZED",
        "message": "valid X-Api-Key required"
    }
}
```


## 5.2 Moderation queue (pending reviews)

```http
GET http://localhost:8081/internal/v1/reviews/pending
X-Api-Key: dev-internal-key
```

**Status: 200 ✅**

```json
{
    "data": {
        "offset": 0,
        "reviews": [
            {
                "id": "rv_3af9dbf37acaff0b282cadc4",
                "productId": "PROD-EVD",
                "customerId": "C-EVD-2",
                "rating": 1,
                "title": "scam product",
                "body": "Do not buy, this is a scam!",
                "verifiedPurchase": false,
                "status": "PENDING",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "moderationNotes": "contains banned term: scam",
                "createdAt": "2026-04-28T22:48:57.276683-07:00",
                "updatedAt": "2026-04-28T22:48:57.276683-07:00"
            }
        ],
        "total": 1
    }
}
```


## 5.3 Approve the pending review

```http
PATCH http://localhost:8081/internal/v1/reviews/rv_3af9dbf37acaff0b282cadc4/moderation
X-Api-Key: dev-internal-key
Content-Type: application/json

{"approve":true,"notes":"false positive from word-list moderator"}
```

**Status: 200 ✅**

```json
{
    "data": {
        "id": "rv_3af9dbf37acaff0b282cadc4",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-2",
        "rating": 1,
        "title": "scam product",
        "body": "Do not buy, this is a scam!",
        "verifiedPurchase": false,
        "status": "APPROVED",
        "helpfulCount": 0,
        "unhelpfulCount": 0,
        "moderationNotes": "false positive from word-list moderator",
        "createdAt": "2026-04-28T22:48:57.276683-07:00",
        "updatedAt": "2026-04-28T22:48:57.766401-07:00"
    }
}
```


## 5.4 Approved review now appears on public list

```http
GET http://localhost:8080/v1/products/PROD-EVD/reviews
```

**Status: 200 ✅**

```json
{
    "data": {
        "nextCursor": "",
        "reviews": [
            {
                "id": "rv_3af9dbf37acaff0b282cadc4",
                "productId": "PROD-EVD",
                "customerId": "C-EVD-2",
                "rating": 1,
                "title": "scam product",
                "body": "Do not buy, this is a scam!",
                "verifiedPurchase": false,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "moderationNotes": "false positive from word-list moderator",
                "createdAt": "2026-04-28T22:48:57.276683-07:00",
                "updatedAt": "2026-04-28T22:48:57.766401-07:00"
            },
            {
                "id": "rv_975682208bf3b32e54b43f5f",
                "productId": "PROD-EVD",
                "customerId": "C-EVD-1",
                "rating": 5,
                "title": "Hackathon-grade",
                "body": "Best product ever tested in this POC.",
                "verifiedPurchase": true,
                "status": "APPROVED",
                "helpfulCount": 0,
                "unhelpfulCount": 0,
                "createdAt": "2026-04-28T22:48:57.066506-07:00",
                "updatedAt": "2026-04-28T22:48:57.066506-07:00"
            }
        ]
    }
}
```

> Public list now shows **2** review(s) — approved review visible ✅


## 6. Helpful votes


## 6.1 Mark a review helpful

```http
POST http://localhost:8080/v1/reviews/rv_975682208bf3b32e54b43f5f/helpful
X-Customer-Id: C-EVD-3
```

**Status: 200 ✅**

```json
{
    "data": {
        "id": "rv_975682208bf3b32e54b43f5f",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-1",
        "rating": 5,
        "title": "Hackathon-grade",
        "body": "Best product ever tested in this POC.",
        "verifiedPurchase": true,
        "status": "APPROVED",
        "helpfulCount": 1,
        "unhelpfulCount": 0,
        "createdAt": "2026-04-28T22:48:57.066506-07:00",
        "updatedAt": "2026-04-28T22:48:57.887202-07:00"
    }
}
```


## 6.2 Mark a review unhelpful

```http
POST http://localhost:8080/v1/reviews/rv_975682208bf3b32e54b43f5f/unhelpful
X-Customer-Id: C-EVD-4
```

**Status: 200 ✅**

```json
{
    "data": {
        "id": "rv_975682208bf3b32e54b43f5f",
        "productId": "PROD-EVD",
        "customerId": "C-EVD-1",
        "rating": 5,
        "title": "Hackathon-grade",
        "body": "Best product ever tested in this POC.",
        "verifiedPurchase": true,
        "status": "APPROVED",
        "helpfulCount": 1,
        "unhelpfulCount": 1,
        "createdAt": "2026-04-28T22:48:57.066506-07:00",
        "updatedAt": "2026-04-28T22:48:57.938753-07:00"
    }
}
```


## 7. Bulk ingest (internal)

```http
POST http://localhost:8081/internal/v1/reviews/bulk
X-Api-Key: dev-internal-key
Content-Type: application/json

[3 reviews for PROD-BULK]
```

**Status: 202 ✅**

```json
{
    "data": {
        "created": [
            "rv_a0e7f6a807f46c6e76581569",
            "rv_61efd64dd072e322bc78b808",
            "rv_51b961af0079f3b8da5efec2"
        ],
        "failed": null
    }
}
```


## 7.1 Force recompute aggregates for PROD-BULK

```http
POST http://localhost:8081/internal/v1/products/PROD-BULK/ratings/recompute
X-Api-Key: dev-internal-key
```

**Status: 200 ✅**

```json
{
    "data": {
        "productId": "PROD-BULK",
        "averageRating": 4,
        "totalReviews": 3,
        "distribution": {
            "1": 0,
            "2": 0,
            "3": 1,
            "4": 1,
            "5": 1
        },
        "updatedAt": "2026-04-28T22:48:58.041903-07:00"
    }
}
```


## 8. Validation errors


## 8.1 Rating out of range

```http
POST http://localhost:8080/v1/products/P/reviews
X-Customer-Id: C-X
Content-Type: application/json

{"rating":6,"title":"x","body":"y"}
```

**Status: 400 ⚠️**

```json
{
    "error": {
        "code": "VALIDATION_FAILED",
        "message": "rating must be between 1 and 5"
    }
}
```


## 8.2 Missing X-Customer-Id on write → 401

```http
POST http://localhost:8080/v1/products/P/reviews  (no auth header)
```

**Status: 401 ⚠️**

```json
{
    "error": {
        "code": "UNAUTHORIZED",
        "message": "X-Customer-Id required"
    }
}
```


## 8.3 Hard delete (internal)

```http
DELETE http://localhost:8081/internal/v1/reviews/rv_3af9dbf37acaff0b282cadc4
X-Api-Key: dev-internal-key
```

**Status: 204 ✅**

```json

```


## 8.4 Fetch deleted review → 404

```http
GET http://localhost:8080/v1/reviews/rv_3af9dbf37acaff0b282cadc4
```

**Status: 404 ⚠️**

```json
{
    "error": {
        "code": "REVIEW_NOT_FOUND",
        "message": "review not found"
    }
}
```


---

*Evidence captured at 2026-04-29T05:48:56Z by capture_evidence.sh*
