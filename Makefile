.PHONY: run test build vet tidy curl-demo

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./... -count=1

vet:
	go vet ./...

tidy:
	go mod tidy

curl-demo:
	./scripts/curl_examples.sh
