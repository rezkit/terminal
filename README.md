# rezkit

A read-only terminal UI for RezKit. Browse bookings,
customers and transactions across your RezKit workspaces without leaving the
terminal.

## Install

Download the binary for your platform from the
[latest release](https://github.com/rezkit/terminal/releases/latest). Each
release has `rezkit-darwin-arm64` (Apple Silicon Macs),
`rezkit-linux-amd64`, `rezkit-linux-arm64` and `rezkit-windows-amd64.exe`.

### macOS and Linux

Pick the file for your machine, make it executable and put it on your `PATH`:

```sh
# macOS (Apple Silicon); on Linux use rezkit-linux-amd64 or rezkit-linux-arm64
curl -fLO https://github.com/rezkit/terminal/releases/latest/download/rezkit-darwin-arm64
chmod +x rezkit-darwin-arm64
sudo mv rezkit-darwin-arm64 /usr/local/bin/rezkit
```

Downloaded files aren't executable until you `chmod +x` them. On macOS,
download with `curl` as above: a binary downloaded in a browser is
quarantined, and macOS will refuse to open it because it isn't signed.

To check a download, fetch `checksums.txt` from the same release and compare
before moving the file, e.g. on Linux:

```sh
curl -fLO https://github.com/rezkit/terminal/releases/latest/download/checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

### Windows

In PowerShell, download the `.exe` and move it to a folder on your `PATH`:

```powershell
Invoke-WebRequest -OutFile rezkit.exe https://github.com/rezkit/terminal/releases/latest/download/rezkit-windows-amd64.exe
```

Windows may warn that the file is from an unrecognised publisher, because it
isn't signed.

### With Go

```sh
go install github.com/rezkit/terminal/cmd/rezkit@latest
```

Builds installed this way report their version as `dev`.

## Getting started

```sh
rezkit --demo   # try it with built-in sample data, no account needed
rezkit login    # sign in to RezKit
rezkit          # browse your workspaces
```

`rezkit login` shows a URL and a code and opens your browser; approve the
sign-in there and the terminal picks it up. You can also sign in from inside
the app. Other commands:

| Command | |
|---|---|
| `rezkit whoami` | Show who you're signed in as |
| `rezkit logout` | Forget stored credentials |
| `rezkit version` | Show the version |
| `rezkit help` | Show usage, including where your config lives |

### Keys

| Key | Action |
|---|---|
| `tab`, `1`–`3` | Switch between Bookings, Customers and Transactions |
| `enter` | Open the selected item |
| `/` | Filter the list · `x` clears the filter |
| `s` / `S` | Change sort column / direction |
| `n` / `p` | Next / previous page |
| `ctrl+r` | Reload |
| `w` | Switch workspace |
| `esc` | Back |
| `?` | Show all keys for the current screen |
| `q` | Quit |

## Configuration

Everything defaults to RezKit production, so most people need no
configuration. Settings live in `config.toml` in the config directory:

| OS | Config directory |
|---|---|
| Linux | `~/.config/rezkit-tui` (or `$XDG_CONFIG_HOME/rezkit-tui`) |
| macOS | `~/Library/Application Support/rezkit-tui` |
| Windows | `%AppData%\rezkit-tui` |

Set `REZKIT_CONFIG_DIR` to use a different directory. The file is optional;
these are the defaults:

```toml
issuer    = "https://auth.rezkit.app/oidc"
client_id = "aeajwp2w42rdv4mcn7jki"
resource  = "https://api.rezkit.app/sales"
endpoint  = "https://api.rezkit.app/api/graphql"
scopes    = []                                     # extra API scopes, if needed
```

Each setting can also be set with an environment variable: `REZKIT_ISSUER`,
`REZKIT_CLIENT_ID`, `REZKIT_RESOURCE`, `REZKIT_ENDPOINT` and `REZKIT_SCOPES`.
If you change `resource`, log in again.

Other environment variables:

| Variable | Effect |
|---|---|
| `REZKIT_LOCALE` | Locale for formatting money, e.g. `en-GB` or `de_DE.UTF-8`. Defaults to your shell's locale (`LC_ALL`, then `LC_MONETARY`, then `LANG`). |
| `REZKIT_NO_BROWSER=1` | Don't open a browser when signing in; the URL and code are still shown. |

Your sign-in is kept in the OS keychain. Where there's no keychain (common on
WSL), it's saved in `token.json` in the config directory, readable only by
you. The app remembers your last workspace in `state.toml` and never rewrites
`config.toml`.

## Development

See [AGENTS.md](AGENTS.md) for building from source, the code layout and the
release process.

## License

MIT. See [LICENSE](LICENSE).
