BIN      := bin/rezkit
# Strip the tag's leading "v" so local builds report versions the same way as releases.
VERSION  ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
LDFLAGS  := -s -w -X main.version=$(VERSION)
SCHEMA   := internal/api/graphql/schema.graphql

# Release targets as GOOS/GOARCH. Keep in sync with builds.targets in .goreleaser.yaml.
CROSS_TARGETS := darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: build run demo test lint generate schema clean cross

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/rezkit

run: build
	./$(BIN)

demo: build
	./$(BIN) --demo

test:
	go test ./...

# golangci-lint is optional; fall back to go vet when it is not installed.
lint:
	@if command -v golangci-lint >/dev/null; then golangci-lint run; else echo "golangci-lint not installed; running go vet"; go vet ./...; fi

# Introspect the live API (requires `rezkit login` and an endpoint in config).
schema: build
	./$(BIN) dev schema -o $(SCHEMA)

# Regenerate the GraphQL client (requires $(SCHEMA)).
generate:
	@test -f $(SCHEMA) || { echo "$(SCHEMA) missing: run 'make schema' first"; exit 1; }
	cd internal/api/graphql && go tool genqlient

# Cross-compile every release target to bin/rezkit-<os>-<arch>[.exe],
# named like the release assets.
cross:
	@set -e; for t in $(CROSS_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
		out=$(BIN)-$$os-$$arch$$ext; echo "$$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $$out ./cmd/rezkit; \
	done

clean:
	rm -rf bin dist
