VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)
PLUGIN_OUT ?= dist/codebridge-plugin.zip

.PHONY: fmt fmt-check test build run-manager run-client run-doctor plugin plugin-web

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

plugin:
	@if [ -z "$(MCP_URL)" ]; then echo "MCP_URL is required, e.g. https://codebridge.example.com/mcp"; exit 2; fi
	go run ./tools/pluginpack --mcp-url "$(MCP_URL)" --out "$(PLUGIN_OUT)"

plugin-web:
	@if [ -z "$(MCP_URL)" ]; then echo "MCP_URL is required, e.g. https://codebridge.example.com/mcp"; exit 2; fi
	@if [ -z "$(APP_ID)" ]; then echo "APP_ID is required; copy the plugin_asdk_app... technical ID from ChatGPT developer mode"; exit 2; fi
	go run ./tools/pluginpack --mcp-url "$(MCP_URL)" --app-id "$(APP_ID)" --out "$(PLUGIN_OUT)"
