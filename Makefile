.PHONY: run run-sqlite test build vet tidy curl-demo evidence

# Auto-detect Go: prefer one on PATH, fall back to the standard /usr/local/go install.
GO := $(shell command -v go 2>/dev/null || ([ -x /usr/local/go/bin/go ] && echo /usr/local/go/bin/go) || echo go)

run:
	$(GO) run ./cmd/server

run-sqlite:
	mkdir -p data
	STORAGE_DRIVER=sqlite STORAGE_PATH=./data/omnimart.db $(GO) run ./cmd/server

build:
	$(GO) build -o bin/server ./cmd/server

test:
	$(GO) test ./... -count=1

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

curl-demo:
	./scripts/curl_examples.sh

evidence:
	./scripts/capture_evidence.sh
