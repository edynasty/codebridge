.PHONY: fmt test build run-manager run-client

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/codebridge-manager ./cmd/manager
	go build -o bin/codebridge-client ./cmd/client

run-manager:
	go run ./cmd/manager

run-client:
	go run ./cmd/client
