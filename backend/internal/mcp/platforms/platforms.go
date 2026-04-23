// Package platforms wires every platform-specific MCP package
// (linkedin-go/mcp, x-go/mcp, …) into agent-setup.
//
// Each platform supplies a [Plugin] that pairs an [mcp.PlatformBinding] (the
// Provider plus a per-request client constructor) with a credential
// [credentials.Validator]. main.go composes the full registry from
// [All] (or any subset) so adding a new platform is one entry in [All] plus
// a new factory below.
//
// Bindings are intentionally NOT auto-registered via init() so that callers
// can opt out of unwanted platforms (tests, narrow deployments) and so the
// dependency graph is explicit at startup.
package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	codegen "github.com/teslashibe/codegen-go"
	codegenmcp "github.com/teslashibe/codegen-go/mcp"
	elevenlabs "github.com/teslashibe/elevenlabs-go"
	elevenmcp "github.com/teslashibe/elevenlabs-go/mcp"
	"github.com/teslashibe/facebook-go/groups"
	facebookmcp "github.com/teslashibe/facebook-go/mcp"
	hn "github.com/teslashibe/hn-go"
	hnmcp "github.com/teslashibe/hn-go/mcp"
	instagram "github.com/teslashibe/instagram-go"
	instagrammcp "github.com/teslashibe/instagram-go/mcp"
	linkedin "github.com/teslashibe/linkedin-go"
	linkedinmcp "github.com/teslashibe/linkedin-go/mcp"
	nextdoor "github.com/teslashibe/nextdoor-go"
	nextdoormcp "github.com/teslashibe/nextdoor-go/mcp"
	producthunt "github.com/teslashibe/producthunt-go"
	producthuntmcp "github.com/teslashibe/producthunt-go/mcp"
	reddit "github.com/teslashibe/reddit-go"
	redditmcp "github.com/teslashibe/reddit-go/mcp"
	redditviral "github.com/teslashibe/redditviral-go"
	redditviralmcp "github.com/teslashibe/redditviral-go/mcp"
	threads "github.com/teslashibe/threads-go"
	threadsmcp "github.com/teslashibe/threads-go/mcp"
	tiktok "github.com/teslashibe/tiktok-go"
	tiktokmcp "github.com/teslashibe/tiktok-go/mcp"
	x "github.com/teslashibe/x-go"
	xmcp "github.com/teslashibe/x-go/mcp"
	xviral "github.com/teslashibe/x-viral-go"
	xviralmcp "github.com/teslashibe/x-viral-go/mcp"

	"github.com/teslashibe/agent-setup/backend/internal/credentials"
	"github.com/teslashibe/agent-setup/backend/internal/mcp"
	"github.com/teslashibe/agent-setup/backend/internal/notifications"
	notificationsmcp "github.com/teslashibe/agent-setup/backend/internal/notifications/mcp"
)

// Plugin pairs a single platform's MCP binding with its credential validator.
type Plugin struct {
	Binding   mcp.PlatformBinding
	Validator credentials.Validator
}

// All returns plugins for every platform compiled into agent-setup.
//
// Order matters only for the settings UI (it determines how connections are
// rendered before any are configured); the registry sorts internally.
func All() []Plugin {
	return []Plugin{
		LinkedIn(),
		X(),
		XViral(),
		Reddit(),
		RedditViral(),
		HackerNews(),
		Facebook(),
		Instagram(),
		TikTok(),
		Threads(),
		ProductHunt(),
		Nextdoor(),
		ElevenLabs(),
		Codegen(),
	}
}

// credentialBlob is the union shape every settings-UI submission can take.
// Platforms only read the fields they need; missing fields produce a
// platform-specific validation error so the caller knows exactly what to fix.
//
// `cookies` is the canonical map shape for browser-extracted multi-cookie
// auth (Facebook, Instagram, TikTok). `cookie` is a single-cookie shorthand
// (LinkedIn li_at, Nextdoor xsrf). `token` is for API-key style platforms
// (ElevenLabs XI-API-Key, ProductHunt developer token).
//
// Extra `extra` is forwarded as-is to platform-specific decoders for fields
// the canonical struct doesn't know about (e.g. CSRF tokens).
type credentialBlob struct {
	Cookie  string            `json:"cookie,omitempty"`
	Cookies map[string]string `json:"cookies,omitempty"`
	Token   string            `json:"token,omitempty"`
	Extra   map[string]string `json:"extra,omitempty"`
}

func parseCredential(raw json.RawMessage) (credentialBlob, error) {
	var c credentialBlob
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("invalid credential JSON: %w", err)
	}
	return c, nil
}

// cookieMap returns Cookies if non-empty; otherwise falls back to a
// single-key map keyed "cookie" so callers can still consume Cookie.
// Returns nil only when neither shape is populated.
func (c credentialBlob) cookieMap() map[string]string {
	if len(c.Cookies) > 0 {
		return c.Cookies
	}
	if c.Cookie != "" {
		return map[string]string{"cookie": c.Cookie}
	}
	return nil
}

// LinkedIn binds linkedin-go.
func LinkedIn() Plugin {
	const platform = "linkedin"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: linkedinmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("linkedin credential missing 'cookies' map (li_at, JSESSIONID) or 'cookie' (li_at)")
				}
				auth := linkedin.Auth{
					LiAt:       firstNonEmpty(cookies["li_at"], cookies["cookie"], cred.Cookie),
					CSRF:       strings.Trim(firstNonEmpty(cookies["JSESSIONID"], cookies["jsessionid"], cred.Extra["csrf"]), `"`),
					JSESSIONID: strings.Trim(firstNonEmpty(cookies["JSESSIONID"], cookies["jsessionid"]), `"`),
				}
				if auth.LiAt == "" {
					return nil, errors.New("linkedin credential missing 'li_at'")
				}
				if auth.CSRF == "" {
					return nil, errors.New("linkedin credential missing JSESSIONID/CSRF")
				}
				return linkedin.New(auth), nil
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"cookie", "cookies"}},
	}
}

// X binds x-go.
func X() Plugin {
	const platform = "x"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: xmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("x credential missing 'cookies' map (auth_token, ct0)")
				}
				return x.New(x.Cookies{
					AuthToken: cookies["auth_token"],
					CT0:       cookies["ct0"],
					Twid:      cookies["twid"],
					KDT:       cookies["kdt"],
				})
			},
		},
		Validator: simpleValidator{platform: platform, requireCookies: []string{"auth_token", "ct0"}},
	}
}

// XViral binds x-viral-go (deterministic scorer; no credentials).
func XViral() Plugin {
	const platform = "xviral"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: xviralmcp.Provider{},
			NewClient: func(_ context.Context, _ json.RawMessage) (any, error) {
				return xviral.New(), nil
			},
		},
		Validator: nullValidator{platform: platform},
	}
}

// Reddit binds reddit-go.
//
// Most reddit endpoints work with just the bearer token (token_v2)
// hitting oauth.reddit.com. PostInsights is the exception: it scrapes
// www.reddit.com/poststats/{id}/, which redirects bearer-only
// requests to /login. So when the credential blob includes the full
// cookies map (as the browser extension and the cred-check helper
// both emit), forward the whole set to reddit-go so the client can
// attach it to www.reddit.com requests.
func Reddit() Plugin {
	const platform = "reddit"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: redditmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				token := firstNonEmpty(cred.Token, cred.Cookie)
				if cookies := cred.Cookies; len(cookies) > 0 && token == "" {
					token = firstNonEmpty(cookies["token_v2"], cookies["token"])
				}
				if token == "" {
					return nil, errors.New("reddit credential missing 'token' (token_v2 cookie)")
				}
				return reddit.New(&reddit.Options{
					Token:   token,
					Cookies: cred.Cookies,
				}), nil
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"token", "cookie", "cookies"}},
	}
}

// RedditViral binds redditviral-go (deterministic scorer; no credentials).
func RedditViral() Plugin {
	const platform = "redditviral"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: redditviralmcp.Provider{},
			NewClient: func(_ context.Context, _ json.RawMessage) (any, error) {
				return redditviral.New(), nil
			},
		},
		Validator: nullValidator{platform: platform},
	}
}

// HackerNews binds hn-go. HN's read-only paths work without auth, but
// commenting/voting requires the `user` cookie from a logged-in session.
func HackerNews() Plugin {
	const platform = "hn"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: hnmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookie := firstNonEmpty(cred.Cookie, cred.Cookies["user"], cred.Token)
				if cookie == "" {
					return nil, errors.New("hn credential missing 'cookie' (user)")
				}
				return hn.New(cookie)
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"cookie", "cookies", "token"}},
	}
}

// Facebook binds facebook-go (groups subpackage).
func Facebook() Plugin {
	const platform = "facebook"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: facebookmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("facebook credential missing 'cookies' map (c_user, xs, fr, datr)")
				}
				return groups.New(groups.Cookies{
					SB:    cookies["sb"],
					DATR:  cookies["datr"],
					CUser: cookies["c_user"],
					XS:    cookies["xs"],
					FR:    cookies["fr"],
					PSL:   cookies["ps_l"],
					PSN:   cookies["ps_n"],
				})
			},
		},
		Validator: simpleValidator{platform: platform, requireCookies: []string{"c_user", "xs"}},
	}
}

// Instagram binds instagram-go.
func Instagram() Plugin {
	const platform = "instagram"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: instagrammcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("instagram credential missing 'cookies' (sessionid, csrftoken, ds_user_id)")
				}
				return instagram.New(instagram.Cookies{
					SessionID: cookies["sessionid"],
					CSRFToken: cookies["csrftoken"],
					DSUserID:  cookies["ds_user_id"],
					Datr:      cookies["datr"],
					Mid:       cookies["mid"],
					IgDid:     cookies["ig_did"],
					Rur:       cookies["rur"],
					IgNrcb:    cookies["ig_nrcb"],
					PsL:       cookies["ps_l"],
					PsN:       cookies["ps_n"],
				})
			},
		},
		Validator: simpleValidator{platform: platform, requireCookies: []string{"sessionid"}},
	}
}

// TikTok binds tiktok-go.
func TikTok() Plugin {
	const platform = "tiktok"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: tiktokmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("tiktok credential missing 'cookies' (sessionid, tt_csrf_token)")
				}
				return tiktok.New(tiktok.Cookies{
					SessionID: cookies["sessionid"],
					SIDtt:     cookies["sid_tt"],
					CSRFToken: cookies["tt_csrf_token"],
					MsToken:   cookies["msToken"],
					TTWid:     cookies["ttwid"],
					OdinTT:    cookies["odin_tt"],
					SIDUcpV1:  cookies["sid_ucp_v1"],
					UIDtt:     cookies["uid_tt"],
					Ttp:       cookies["_ttp"],
				})
			},
		},
		Validator: simpleValidator{platform: platform, requireCookies: []string{"sessionid", "tt_csrf_token"}},
	}
}

// Threads binds threads-go. Threads supports two parallel auth schemes:
//
//   - Cookies (sessionid + csrftoken) — required for read endpoints
//     (timeline, search, profile, hashtags, …).
//   - Bearer token (IGT:2:…) + UserID — required for write endpoints
//     (post, like, follow, …).
//
// Most users will paste both. We accept either subset and return the most
// privileged client we can construct; tools that need write auth on a
// read-only client will surface a clear error from threads-go itself.
func Threads() Plugin {
	const platform = "threads"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: threadsmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				hasCookies := cookies != nil && cookies["sessionid"] != "" && cookies["csrftoken"] != ""
				token := firstNonEmpty(cred.Token, cred.Extra["bearer"])
				userID := cred.Extra["user_id"]
				if userID == "" && cookies != nil {
					userID = cookies["ds_user_id"]
				}
				hasBearer := token != "" && userID != ""
				if !hasCookies && !hasBearer {
					return nil, errors.New("threads credential needs cookies (sessionid+csrftoken) or token+user_id")
				}
				ck := threads.Cookies{}
				if cookies != nil {
					ck = threads.Cookies{
						SessionID: cookies["sessionid"],
						CSRFToken: cookies["csrftoken"],
						DSUserID:  cookies["ds_user_id"],
						Mid:       cookies["mid"],
						IgDid:     cookies["ig_did"],
					}
				}
				auth := threads.Auth{
					Token:    token,
					UserID:   userID,
					DeviceID: cred.Extra["device_id"],
				}
				switch {
				case hasCookies && hasBearer:
					return threads.NewFull(ck, auth)
				case hasBearer:
					return threads.NewWithAuth(auth)
				default:
					return threads.New(ck)
				}
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"cookies", "token", "extra"}},
	}
}

// ProductHunt binds producthunt-go.
func ProductHunt() Plugin {
	const platform = "producthunt"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: producthuntmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					cookies = map[string]string{}
				}
				creds := producthunt.Credentials{
					DeveloperToken: cred.Token,
					ClientID:       cred.Extra["client_id"],
					ClientSecret:   cred.Extra["client_secret"],
					Session:        cookies["_producthunt_session_production"],
					CFClearance:    cookies["cf_clearance"],
					CFBM:           cookies["__cf_bm"],
					CSRFToken:      firstNonEmpty(cred.Extra["csrf_token"], cookies["csrf_token"]),
				}
				if creds.DeveloperToken == "" && creds.ClientID == "" && creds.Session == "" {
					return nil, errors.New("producthunt credential needs 'token' (developer token), client_id+secret, or session cookies")
				}
				return producthunt.New(creds)
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"token", "cookies", "extra"}},
	}
}

// Nextdoor binds nextdoor-go.
//
// Cookie schema note: Nextdoor renamed their browser auth cookies in late
// 2025. The current names are `csrftoken` (X-CSRFToken header) and
// `ndbr_at` (Nextdoor browser access token, HttpOnly). The legacy
// `xsrf` / `access_token` names are still accepted as fallbacks so older
// stored credentials keep working while operators rotate.
func Nextdoor() Plugin {
	const platform = "nextdoor"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: nextdoormcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cookies := cred.cookieMap()
				if cookies == nil {
					return nil, errors.New("nextdoor credential missing 'cookies' (csrftoken, ndbr_at)")
				}
				csrf := firstNonEmpty(cookies["csrftoken"], cookies["xsrf"], cred.Extra["csrf_token"])
				access := firstNonEmpty(cookies["ndbr_at"], cookies["access_token"], cred.Token)
				if csrf == "" || access == "" {
					return nil, errors.New("nextdoor credential missing 'csrftoken' and/or 'ndbr_at' cookies (legacy 'xsrf'/'access_token' also accepted)")
				}
				return nextdoor.New(nextdoor.Auth{
					CSRFToken:   csrf,
					AccessToken: access,
				})
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"cookies", "token"}},
	}
}

// ElevenLabs binds elevenlabs-go (XI-API-Key auth).
func ElevenLabs() Plugin {
	const platform = "elevenlabs"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: elevenmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				if cred.Token == "" {
					return nil, errors.New("elevenlabs credential missing 'token' (XI-API-Key)")
				}
				return elevenlabs.New(cred.Token)
			},
		},
		Validator: simpleValidator{platform: platform, requireOneOf: []string{"token"}},
	}
}

// Codegen binds codegen-go. The codegen wrapper shells out to a locally
// installed `claude` binary (or generic CLI) and does not need user-supplied
// credentials; auth is whatever the binary itself uses (e.g. `claude login`).
func Codegen() Plugin {
	const platform = "codegen"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider: codegenmcp.Provider{},
			NewClient: func(_ context.Context, raw json.RawMessage) (any, error) {
				cred, err := parseCredential(raw)
				if err != nil {
					return nil, err
				}
				cfg := codegen.Config{
					Type:  firstNonEmpty(cred.Extra["type"], "claude-code"),
					Model: cred.Extra["model"],
				}
				return codegen.NewAgent(cfg)
			},
		},
		Validator: nullValidator{platform: platform},
	}
}

// Notifications binds the internal notifications platform: a per-user view
// of device-captured notifications used by the daily-rollup tools.
//
// Unlike every other plugin in this file, Notifications is *not* in
// All() — agent-setup's main.go appends it conditionally on
// cfg.NotificationsEnabled so forks that don't ship the Android capture
// feature pay zero overhead. The binding sets NoCredentials=true because
// data is pushed by the user's own authenticated device, not pulled from
// an external service that needs cookies/tokens.
//
// The per-request user ID lives on ctx via mcp.UserIDFromContext (set by
// the MCP server before calling NewClient).
func Notifications(svc *notifications.Service) Plugin {
	const platform = "notifications"
	return Plugin{
		Binding: mcp.PlatformBinding{
			Provider:      notificationsmcp.Provider{},
			NoCredentials: true,
			NewClient: func(ctx context.Context, _ json.RawMessage) (any, error) {
				userID := mcp.UserIDFromContext(ctx)
				if userID == "" {
					return nil, errors.New("notifications: missing authenticated user id on MCP request context")
				}
				return &notificationsmcp.Client{Svc: svc, UserID: userID}, nil
			},
		},
		Validator: nullValidator{platform: platform},
	}
}

// simpleValidator is a generic credential-shape validator. It does not test
// the credential against the upstream service (that happens lazily at
// tool-call time); it only enforces structural completeness.
type simpleValidator struct {
	platform       string
	requireField   string
	requireOneOf   []string
	requireCookies []string
}

func (v simpleValidator) Platform() string { return v.platform }

func (v simpleValidator) Validate(raw json.RawMessage) error {
	cred, err := parseCredential(raw)
	if err != nil {
		return err
	}
	if v.requireField != "" && !hasField(cred, v.requireField) {
		return fmt.Errorf("missing required field %q", v.requireField)
	}
	if len(v.requireOneOf) > 0 {
		for _, f := range v.requireOneOf {
			if hasField(cred, f) {
				goto cookiesCheck
			}
		}
		return fmt.Errorf("missing one of: %s", strings.Join(v.requireOneOf, ", "))
	}
cookiesCheck:
	for _, name := range v.requireCookies {
		if cred.Cookies[name] == "" {
			return fmt.Errorf("cookies map missing %q", name)
		}
	}
	return nil
}

func hasField(c credentialBlob, field string) bool {
	switch field {
	case "cookie":
		return strings.TrimSpace(c.Cookie) != ""
	case "token":
		return strings.TrimSpace(c.Token) != ""
	case "cookies":
		return len(c.Cookies) > 0
	case "extra":
		return len(c.Extra) > 0
	}
	return false
}

// nullValidator accepts any (or empty) credential. Used by platforms that
// don't require user-provided auth.
type nullValidator struct{ platform string }

func (v nullValidator) Platform() string                 { return v.platform }
func (v nullValidator) Validate(_ json.RawMessage) error { return nil }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
