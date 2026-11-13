# rezkit

A read-only terminal UI for RezKit, built with Go and Bubble Tea v2.

## Quick start

```sh
go build -o bin/rezkit ./cmd/rezkit   # or: make build
./bin/rezkit --demo                   # sample data, no login needed
./bin/rezkit login                    # device-code login via Logto
./bin/rezkit                          # browse your organizations
```

Keys: `tab`/`1-3` switch between Bookings, Customers and Transactions · `enter` open ·
`/` filter · `x` clear filter · `s`/`S` sort column/direction ·
`n`/`p` page · `ctrl+r` reload · `w` switch workspace · `esc` back · `?` help · `q` quit.

## Configuration

`~/.config/rezkit-tui/config.toml` (override the directory with `REZKIT_CONFIG_DIR`).
Everything defaults to RezKit production, so the file is optional:

```toml
issuer    = "https://auth.rezkit.app/oidc"
client_id = "aeajwp2w42rdv4mcn7jki"
resource  = "https://api.rezkit.app/sales"        # Logto API resource indicator
endpoint  = "https://api.rezkit.app/api/graphql"
scopes    = []                                     # extra API scopes, if needed
```

Each key has a `REZKIT_*` environment override (`REZKIT_ISSUER`, `REZKIT_CLIENT_ID`,
`REZKIT_RESOURCE`, `REZKIT_ENDPOINT`, `REZKIT_SCOPES`). Money is formatted for the
shell locale (`LC_ALL`, then `LC_MONETARY`, then `LANG`); set `REZKIT_LOCALE`
(e.g. `en-GB`, `de_DE.UTF-8`) to override it. `REZKIT_NO_BROWSER=1`
stops the app opening a browser at login (the URL and code are always shown).

Tokens are kept in the OS keychain, or in a `0600` `token.json` in the config
directory when no keychain is available (typical on WSL). Refresh tokens are
rotated and saved automatically. If you change `resource`, log in again.

The last-used workspace is stored in `state.toml`; the app never rewrites
`config.toml`.

## Layout

| Path | Purpose |
|---|---|
| `cmd/rezkit` | Entry point and subcommands |
| `internal/auth` | Device authorization grant with PKCE, refresh with resource indicator, token storage |
| `internal/api` | `Service` interface and plain types used by the UI; `RezKit` adapter; `Fake` for demo/tests |
| `internal/api/graphql` | Schema, queries and the genqlient-generated client |
| `internal/api/scalar` | Go types for custom scalars (`Timestamp`) |
| `internal/ui` | Root app: screen stack, modal layer, header/status/help |
| `internal/ui/components` | Server-paginated, server-sorted `Table`; static `Grid`; text helpers |
| `internal/ui/screens` | Login, workspace picker, home sections (bookings, customers, transactions lists with filter modals), booking screen (tabs; invoice/itinerary), customer screen (tabs; bookings open the booking), generic detail view (unused) |

## GraphQL client

Workspaces are RezKit organizations; the selected organization is passed as
the `organizationId` argument. Bookings, customers and transactions use Relay-style
connections, so paging is cursor-based and sorting is server-side (bookings:
reference, description, created; customers: family name, email, city, created).
The transactions query takes no sort argument, so that list has a fixed order
and opening a transaction shows its booking's payments.

To change what is fetched:

1. Edit `internal/api/graphql/queries/*.graphql`.
2. `go generate ./internal/api/graphql` (or `make generate`).
3. Map the generated types to the `api` types in `internal/api/rezkit*.go`.

Refresh the schema with `rezkit dev schema` (introspection works without
logging in). The server returns fields in no fixed order, so a refreshed
schema may show reordering noise in diffs.

The API reports fields the caller can't read as `null` with a warning, not
an error; the adapter turns that into "access denied (try logging in again)".
