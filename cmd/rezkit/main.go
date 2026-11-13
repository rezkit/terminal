// Command rezkit is a terminal UI for browsing RezKit data.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"
	_ "time/tzdata" // stop timezones on systems without a zoneinfo database

	tea "charm.land/bubbletea/v2"
	"golang.org/x/oauth2"

	"github.com/rezkit/terminal/internal/api"
	"github.com/rezkit/terminal/internal/auth"
	"github.com/rezkit/terminal/internal/browser"
	"github.com/rezkit/terminal/internal/config"
	"github.com/rezkit/terminal/internal/ui"
	"github.com/rezkit/terminal/internal/ui/screens"
)

var version = "dev"

const usage = `rezkit — terminal UI for RezKit

Usage:
  rezkit [--demo]    start the TUI (--demo uses built-in sample data)
  rezkit login       log in with a device code
  rezkit logout      forget stored credentials
  rezkit whoami      show the logged-in user
  rezkit dev schema [-o file]
                         introspect the API and write the GraphQL schema
  rezkit version

Configuration: %s
(environment overrides: REZKIT_ISSUER, REZKIT_CLIENT_ID, REZKIT_RESOURCE, REZKIT_LOCALE,
 REZKIT_SCOPES, REZKIT_ENDPOINT, REZKIT_CONFIG_DIR,
 REZKIT_NO_BROWSER)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := ""
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "":
		fs := flag.NewFlagSet("rezkit", flag.ContinueOnError)
		demo := fs.Bool("demo", false, "use built-in sample data instead of the API")
		fs.Usage = func() { fmt.Fprintf(os.Stderr, usage, cfg.Path()) }
		if err := fs.Parse(args); err != nil {
			return err
		}
		return runTUI(ctx, cfg, *demo)
	case "login":
		return cmdLogin(ctx, cfg)
	case "logout":
		return cmdLogout(cfg)
	case "whoami":
		return cmdWhoami(cfg)
	case "dev":
		return cmdDev(ctx, cfg, args)
	case "version":
		fmt.Println(version)
		return nil
	case "help":
		fmt.Printf(usage, cfg.Path())
		return nil
	default:
		return fmt.Errorf("unknown command %q (try 'rezkit help')", cmd)
	}
}

// session bundles what an authenticated run needs.
type session struct {
	auth  *auth.Authenticator
	store auth.Store
}

func newSession(ctx context.Context, cfg *config.Config) (*session, error) {
	if err := cfg.ValidateAuth(); err != nil {
		return nil, err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	a, err := auth.New(dctx, auth.Options{
		Issuer: cfg.Issuer, ClientID: cfg.ClientID, Resource: cfg.Resource, Scopes: cfg.Scopes,
	})
	if err != nil {
		return nil, err
	}
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	return &session{auth: a, store: auth.NewStore(cfg.ClientID, dir)}, nil
}

// tokenSource returns a refreshing source for the stored token, or
// auth.ErrNoToken / auth.ErrReauthRequired.
func (s *session) tokenSource() (oauth2.TokenSource, *oauth2.Token, error) {
	tok, err := s.store.Load()
	if err != nil {
		return nil, nil, err
	}
	ts := s.auth.TokenSource(tok, s.store)
	fresh, err := ts.Token()
	if err != nil {
		return nil, nil, err
	}
	return ts, fresh, nil
}

func (s *session) login(ctx context.Context, show func(auth.DeviceCode)) error {
	tok, err := s.auth.Login(ctx, show)
	if err != nil {
		return err
	}
	return s.store.Save(tok)
}

func cmdLogin(ctx context.Context, cfg *config.Config) error {
	s, err := newSession(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.Resource == "" {
		fmt.Println("warning: no API resource configured; the token will not be accepted by the RezKit API.\n" +
			"Set `resource` in " + cfg.Path() + " (or REZKIT_RESOURCE) and log in again.")
	}
	err = s.login(ctx, func(dc auth.DeviceCode) {
		url := dc.VerificationURIComplete
		if url == "" {
			url = dc.VerificationURI
		}
		fmt.Printf("Open %s and confirm the code: %s\n", url, dc.UserCode)
		if err := browser.Open(url); err != nil {
			fmt.Println("(couldn't open a browser automatically)")
		}
		fmt.Println("Waiting for approval…")
	})
	if err != nil {
		return err
	}
	tok, _ := s.store.Load()
	if name := auth.DisplayName(tok); name != "" {
		fmt.Println("Logged in as", name)
	} else {
		fmt.Println("Logged in.")
	}
	return nil
}

func cmdLogout(cfg *config.Config) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := auth.NewStore(cfg.ClientID, dir).Delete(); err != nil {
		return err
	}
	fmt.Println("Logged out.")
	return nil
}

func cmdWhoami(cfg *config.Config) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	tok, err := auth.NewStore(cfg.ClientID, dir).Load()
	if err != nil {
		return err
	}
	name := auth.DisplayName(tok)
	if name == "" {
		name = "(unknown user)"
	}
	fmt.Printf("%s\naccess token expires %s\n", name, tok.Expiry.Local().Format(time.RFC1123))
	return nil
}

func cmdDev(ctx context.Context, cfg *config.Config, args []string) error {
	if len(args) == 0 || args[0] != "schema" {
		return errors.New("usage: rezkit dev schema [-o file]")
	}
	fs := flag.NewFlagSet("dev schema", flag.ContinueOnError)
	out := fs.String("o", filepath.Join("internal", "api", "graphql", "schema.graphql"), "output file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := cfg.ValidateAPI(); err != nil {
		return err
	}
	// Introspection is allowed anonymously; send a token only if we have one.
	var ts oauth2.TokenSource
	if s, err := newSession(ctx, cfg); err == nil {
		if src, _, err := s.tokenSource(); err == nil {
			ts = src
		}
	}
	sdl, err := api.FetchSchema(ctx, cfg.Endpoint, ts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(sdl), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d bytes)\n", *out, len(sdl))
	return nil
}

func runTUI(ctx context.Context, cfg *config.Config, demo bool) error {
	api.SetLocale(api.LocaleFromEnv())
	var (
		svc      api.Service
		observed *api.Observed
		start    ui.Screen
		user     string
	)
	state, err := config.LoadState()
	if err != nil {
		return err
	}
	saveWorkspace := func(w api.Workspace) {
		state.LastWorkspace = w.ID
		_ = state.Save() // best effort: only affects the next start
	}
	picker := func() ui.Screen { return screens.NewWorkspaces(svc, "", saveWorkspace) }

	if demo {
		fake := api.NewFake()
		fake.Latency = 250 * time.Millisecond
		observed = api.Observe(fake)
		svc, user = observed, "demo"
		start = screens.NewWorkspaces(svc, "", nil)
		picker = func() ui.Screen { return screens.NewWorkspaces(svc, "", nil) }
	} else {
		s, err := newSession(ctx, cfg)
		if err != nil {
			return err
		}
		// The token is read from the store lazily, so a login completed on the
		// TUI's login screen takes effect without a restart.
		observed = api.Observe(api.NewRezKit(cfg.Endpoint, api.NewHTTPClient(s.auth.StoreTokenSource(s.store))))
		svc = observed
		resume := func() ui.Screen { return screens.NewWorkspaces(svc, state.LastWorkspace, saveWorkspace) }
		_, tok, err := s.tokenSource()
		switch {
		case err == nil:
			user = auth.DisplayName(tok)
			start = resume()
		case errors.Is(err, auth.ErrNoToken), errors.Is(err, auth.ErrReauthRequired):
			start = screens.NewLogin(s.login, resume)
		default:
			return err
		}
	}

	app := ui.NewApp(ui.Routes{
		WorkspacePicker: picker,
		Home:            func(api.Workspace) ui.Screen { return screens.NewHome(svc) },
	}, start, user)
	p := tea.NewProgram(app, tea.WithContext(ctx))
	// Service calls only run inside commands, never on the event loop, so
	// Send (which waits for the loop) cannot deadlock here.
	observed.SetHooks(api.Hooks{
		Started:  func() { p.Send(ui.RequestStartedMsg{}) },
		Finished: func(d time.Duration) { p.Send(ui.RequestFinishedMsg{Elapsed: d}) },
	})
	_, err = p.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	return err
}
