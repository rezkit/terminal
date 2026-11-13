# AGENTS.md

Notes for anyone (human or agent) working on the code. User-facing docs are
in [README.md](README.md).

`rezkit` is a read-only terminal UI for RezKit, written in Go with Bubble Tea
v2 (`charm.land/*/v2`). The module is `github.com/rezkit/terminal` and the
command is `cmd/rezkit`.

## Build and test

| Command | |
|---|---|
| `make build` | Build `bin/rezkit` (version from `git describe`, leading `v` dropped) |
| `make demo` | Build and run with sample data (`--demo`, no login) |
| `make test` | `go test ./...` |
| `make lint` | `golangci-lint run`, or `go vet` when it isn't installed |
| `make cross` | Build every release target into `bin/rezkit-<os>-<arch>[.exe]` |
| `make generate` | Regenerate the GraphQL client |
| `make schema` | Refresh the GraphQL schema from the live API |

Builds use `CGO_ENABLED=0`, so binaries are static and cross-compile without
a C toolchain.

## Layout

| Path | Purpose |
|---|---|
| `cmd/rezkit` | Entry point and subcommands |
| `internal/auth` | Device authorization grant with PKCE, refresh with resource indicator, token storage |
| `internal/config` | `config.toml` and `state.toml` in the user config directory |
| `internal/api` | `Service` interface and plain types used by the UI; `RezKit` adapter; `Fake` for demo and tests; `Observed` wrapper that reports request activity |
| `internal/api/graphql` | Schema, queries and the genqlient-generated client |
| `internal/api/scalar` | Go types for custom scalars (`Timestamp`) |
| `internal/ui` | Root app: screen stack, modal layer, header, status bar and help |
| `internal/ui/components` | Server-paginated, server-sorted `Table`; static `Grid`; text helpers |
| `internal/ui/screens` | Login, workspace picker, home sections (bookings, customers and transactions lists with filter modals), booking screen (tabs; invoice and itinerary), customer screen (tabs; bookings open the booking), generic detail view (unused) |
| `internal/ui/theme` | Shared Lip Gloss styles |

## GraphQL client

Workspaces are RezKit organizations; the selected organization is passed as
the `organizationId` argument. Bookings, customers and transactions use
Relay-style connections, so paging is cursor-based and sorting is server-side
(bookings: reference, description, created; customers: family name, email,
city, created). The transactions query takes no sort argument, so that list
has a fixed order, and opening a transaction shows its booking's payments.

To change what is fetched:

1. Edit `internal/api/graphql/queries/*.graphql`.
2. `go generate ./internal/api/graphql` (or `make generate`).
3. Map the generated types to the `api` types in `internal/api/rezkit*.go`.

`generated.go` is committed; don't edit it by hand.

Refresh the schema with `rezkit dev schema` (introspection works without
logging in). The server returns fields in no fixed order, so a refreshed
schema may show reordering noise in diffs.

The API reports fields the caller can't read as `null` with a warning, not
an error; the adapter turns that into "access denied (try logging in again)".

## Tests

UI tests in `internal/ui/screens` drive the whole app through a synchronous
harness (`flow_test.go`) against `api.Fake` with zero latency. To test
behaviour the shared fake doesn't cover, embed `*api.Fake` in a small test
type and override the method you need, rather than changing the fake.

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`: `go vet`, the tests,
then GoReleaser (version pinned in the workflow). It publishes bare binaries
(no archives) named `rezkit-<os>-<arch>`, plus `checksums.txt` and `LICENSE`,
to a GitHub release. Tags with a pre-release suffix (`v1.2.0-rc.1`) become
pre-releases.

Targets are darwin/arm64, linux/amd64, linux/arm64 and windows/amd64. They're
listed in both `.goreleaser.yaml` (`builds.targets`) and the `Makefile`
(`CROSS_TARGETS`); keep the two in sync.

## Conventions

- The config directory and keyring service are named `rezkit-tui`, not
  `rezkit`, on purpose: the plain `rezkit` name is kept free for something
  official. Don't rename them.
- In GitHub workflows, reference actions by version tag (`@v7`), not by
  commit SHA.
