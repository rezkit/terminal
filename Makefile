BIN      := bin/rezkit-tui
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
SCHEMA   := internal/api/graphql/schema.graphql

.PHONY: build run demo test lint generate schema clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/rezkit-tui

run: build
	./$(BIN)

demo: build
	./$(BIN) --demo

test:
	go test ./...

# golangci-lint is optional; fall back to go vet when it is not installed.
lint:
	@if command -v golangci-lint >/dev/null; then golangci-lint run; else echo "golangci-lint not installed; running go vet"; go vet ./...; fi

# Introspect the live API (requires `rezkit-tui login` and an endpoint in config).
schema: build
	./$(BIN) dev schema -o $(SCHEMA)

# Regenerate the GraphQL client (requires $(SCHEMA)).
generate:
	@test -f $(SCHEMA) || { echo "$(SCHEMA) missing: run 'make schema' first"; exit 1; }
	cd internal/api/graphql && go tool genqlient

clean:
	rm -rf bin dist
