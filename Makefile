VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: fmt fmt-check test build run-manager run-client

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@files="$$(gofmt -l ./cmd ./internal)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required:"; \
		echo "$$files"; \
		gofmt -d $$files; \
		exit 1; \
	fi

test:
	go test ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/codebridge-manager ./cmd/manager
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/codebridge-client ./cmd/client

run-manager:
	go run ./cmd/manager

run-client:
	go run ./cmd/client
