package invites_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/invites"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// fakeAuth installs synthetic user_id locals so the spec-shaped routes can be
// exercised without booting magic-link middleware.
func fakeAuth(userID string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apperrors.SetUserID(c, userID)
		return c.Next()
	}
}

// newAcceptApp wires the public preview routes and the auth-gated accept
// routes so the test can hit both the spec-shaped (`/:token`) and legacy
// (`?token=` / `/accept`) variants from a single Fiber instance.
func newAcceptApp(t *testing.T, svc *invites.Service, asUserID string) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: apperrors.FiberHandler})

	teamsStore := teams.NewStore(pool)
	teamsSvc := teams.NewService(teamsStore, 25)
	mw := teams.NewMiddleware(teamsSvc)
	h := invites.NewHandler(svc, mw)

	h.MountPublicAPIRoutes(app)
	api := app.Group("/api", fakeAuth(asUserID))
	h.MountAuthAPIRoutes(api)
	return app
}

func httpDo(t *testing.T, app *fiber.App, method, path string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func httpDoBody(t *testing.T, app *fiber.App, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody
}

// TestPublicPreviewByPath asserts the spec route `GET /api/invites/:token` is
// (a) public — no Authorization header — and (b) returns the preview JSON.
func TestPublicPreviewByPath(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "PublicPreview")
	inv, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "preview@local.test", teams.RoleMember)
	if err != nil {
		t.Fatal(err)
	}

	// asUserID is empty — the public route does not require auth, so this
	// proves the preview is reachable without a bearer token.
	app := newAcceptApp(t, svc, "")
	resp, body := httpDo(t, app, http.MethodGet, "/api/invites/"+inv.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var preview struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &preview); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(preview.Email, "preview@local.test") {
		t.Fatalf("unexpected preview body: %s", body)
	}
}

// TestPublicPreviewLegacyAlias proves the query-token alias keeps working
// for any client built against an earlier draft of the API.
func TestPublicPreviewLegacyAlias(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "LegacyPreview")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "legacy@local.test", teams.RoleMember)

	app := newAcceptApp(t, svc, "")
	resp, _ := httpDo(t, app, http.MethodGet, "/api/invites/preview?token="+inv.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("legacy preview: expected 200, got %d", resp.StatusCode)
	}
}

// TestAcceptByPath_HappyPath exercises the spec route
// `POST /api/invites/:token/accept` end-to-end.
func TestAcceptByPath_HappyPath(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "AcceptByPath")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "joiner@local.test", teams.RoleMember)
	joiner := makeUser(t, authSvc, "joiner@local.test")

	app := newAcceptApp(t, svc, joiner.ID)
	resp, body := httpDoBody(t, app, http.MethodPost,
		"/api/invites/"+inv.Token+"/accept", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("accept by path: expected 200, got %d: %s", resp.StatusCode, body)
	}
	var membership teams.Membership
	if err := json.Unmarshal(body, &membership); err != nil {
		t.Fatal(err)
	}
	if membership.Team.ID != team.ID || membership.Role != teams.RoleMember {
		t.Fatalf("membership mismatch: %+v", membership)
	}
}

// TestAcceptByLegacyAlias verifies the legacy POST /api/invites/accept body
// shape (`{token}`) still works.
func TestAcceptByLegacyAlias(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "AcceptLegacy")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "legacy-joiner@local.test", teams.RoleMember)
	joiner := makeUser(t, authSvc, "legacy-joiner@local.test")

	app := newAcceptApp(t, svc, joiner.ID)
	resp, body := httpDoBody(t, app, http.MethodPost,
		"/api/invites/accept", map[string]string{"token": inv.Token})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("accept legacy: expected 200, got %d: %s", resp.StatusCode, body)
	}
}

// TestPreviewRevokedReturnsGone ensures the spec's 410 response is preserved
// under the new path-shaped route.
func TestPreviewRevokedReturnsGone(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "RevokedPreview")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "revoked@local.test", teams.RoleMember)
	if err := svc.Revoke(t.Context(), team.ID, inv.ID); err != nil {
		t.Fatal(err)
	}

	app := newAcceptApp(t, svc, "")
	resp, _ := httpDo(t, app, http.MethodGet, "/api/invites/"+inv.Token)
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 for revoked invite, got %d", resp.StatusCode)
	}
}

// TestConcurrentAccept verifies the spec invariant: "two simultaneous accepts
// of the same token — exactly one succeeds." Backed by SELECT … FOR UPDATE
// inside teams.Store.ConsumeInvite. Run with -race for full confidence.
func TestConcurrentAccept(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "RaceTest")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "race@local.test", teams.RoleMember)
	joiner := makeUser(t, authSvc, "race@local.test")

	const attempts = 10
	var (
		successes atomic.Int32
		conflicts atomic.Int32
		errs      = make(chan error, attempts)
		wg        sync.WaitGroup
		start     = make(chan struct{})
	)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := svc.AcceptByToken(t.Context(), joiner.ID, inv.Token)
			if err == nil {
				successes.Add(1)
				return
			}
			if errors.Is(err, apperrors.ErrInviteAlreadyAccepted) {
				conflicts.Add(1)
				return
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected accept error: %v", err)
	}
	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 successful accept, got %d", got)
	}
	if conflicts.Load() < attempts-1 {
		t.Fatalf("expected at least %d conflicts, got %d", attempts-1, conflicts.Load())
	}
}

// TestExpiredAcceptByPath proves the spec status code (410 ErrInviteExpired)
// is returned for an expired token via the new path route.
func TestExpiredAcceptByPath(t *testing.T) {
	authSvc := auth.NewService(pool)
	teamsSvc := teams.NewService(teams.NewStore(pool), 25)
	sender := &recordingSender{}
	svc := invites.NewService(invites.Config{
		AppName:   "Expired",
		AppURL:    "https://app.test",
		InviteTTL: time.Millisecond,
	}, teamsSvc, authSvc, sender)

	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "ExpiredTeam")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "stale@local.test", teams.RoleMember)
	time.Sleep(5 * time.Millisecond)
	joiner := makeUser(t, authSvc, "stale@local.test")

	app := newAcceptApp(t, svc, joiner.ID)
	resp, body := httpDoBody(t, app, http.MethodPost,
		"/api/invites/"+inv.Token+"/accept", nil)
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 for expired invite, got %d body=%s", resp.StatusCode, body)
	}
}
