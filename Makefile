VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: fmt fmt-check test build run-manager run-client run-doctor

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
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/codebridge-doctor ./cmd/doctor

run-manager:
	go run ./cmd/manager

run-client:
	go run ./cmd/client

run-doctor:
	go run ./cmd/doctor
