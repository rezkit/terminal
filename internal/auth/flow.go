package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Options configure an Authenticator.
type Options struct {
	Issuer     string
	ClientID   string
	Resource   string   // Logto API resource indicator; empty = no API audience
	Scopes     []string // extra API scopes
	HTTPClient *http.Client
}

// Authenticator runs the device-code login and refreshes tokens.
type Authenticator struct {
	opts  Options
	disc  *Discovery
	oauth *oauth2.Config
	http  *http.Client
}

// DeviceCode is what the user needs to complete login in a browser.
type DeviceCode struct {
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string // includes the user code; may be empty
	Expiry                  time.Time
}

// New performs OIDC discovery and prepares the OAuth client config.
func New(ctx context.Context, opts Options) (*Authenticator, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	disc, err := Discover(ctx, opts.HTTPClient, opts.Issuer)
	if err != nil {
		return nil, err
	}
	if disc.DeviceAuthorizationEndpoint == "" {
		return nil, errors.New("identity provider does not advertise a device authorization endpoint")
	}
	scopes := append([]string{"openid", "offline_access", "profile", "email"}, opts.Scopes...)
	return &Authenticator{
		opts: opts,
		disc: disc,
		http: opts.HTTPClient,
		oauth: &oauth2.Config{
			ClientID: opts.ClientID,
			Endpoint: oauth2.Endpoint{
				AuthURL:       disc.AuthorizationEndpoint,
				DeviceAuthURL: disc.DeviceAuthorizationEndpoint,
				TokenURL:      disc.TokenEndpoint,
				AuthStyle:     oauth2.AuthStyleInParams, // public (native) client
			},
			Scopes: scopes,
		},
	}, nil
}

// Discovery returns the provider metadata.
func (a *Authenticator) Discovery() *Discovery { return a.disc }

func (a *Authenticator) resourceOpts() []oauth2.AuthCodeOption {
	if a.opts.Resource == "" {
		return nil
	}
	return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("resource", a.opts.Resource)}
}

// Login runs the OAuth 2.0 Device Authorization Grant (RFC 8628) with PKCE.
// show is called once with the code the user must confirm in a browser; Login
// then polls until the user approves, denies, the code expires, or ctx ends.
func (a *Authenticator) Login(ctx context.Context, show func(DeviceCode)) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.http)
	verifier := oauth2.GenerateVerifier()

	da, err := a.oauth.DeviceAuth(ctx, append(a.resourceOpts(), oauth2.S256ChallengeOption(verifier))...)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	if show != nil {
		show(DeviceCode{
			UserCode:                da.UserCode,
			VerificationURI:         da.VerificationURI,
			VerificationURIComplete: da.VerificationURIComplete,
			Expiry:                  da.Expiry,
		})
	}

	tok, err := a.oauth.DeviceAccessToken(ctx, da, append(a.resourceOpts(), oauth2.VerifierOption(verifier))...)
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			switch re.ErrorCode {
			case "access_denied":
				return nil, errors.New("login was denied in the browser")
			case "expired_token":
				return nil, errors.New("login code expired; please try again")
			}
		}
		return nil, fmt.Errorf("waiting for device login: %w", err)
	}
	return tok, nil
}
