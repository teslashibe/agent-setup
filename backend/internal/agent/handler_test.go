package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/agent"
	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/config"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// stubMW installs synthetic auth + team locals so we can exercise the agent
// handler without booting the full middleware chain.
func stubMW(userID, teamID, role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apperrors.SetUserID(c, userID)
		apperrors.SetTeamID(c, teamID)
		apperrors.SetTeamRole(c, role)
		return c.Next()
	}
}

// stubService is a minimal Handler-compatible Service that exposes the real
// Store but skips the Anthropic client (which can't be constructed in tests).
// We assemble it via reflection-free unsafe? No — agent.Handler holds *Service
// directly. We can build a Service-shaped wrapper instead.
//
// For these tests we only exercise endpoints that touch Store(), so we need a
// real *agent.Service object. agent.NewService refuses to construct without an
// Anthropic key — but we can fake the env for the duration of the test since
// the stubbed handler never calls Run/CreateSession (which require Anthropic).
func newAgentService(t *testing.T) *agent.Service {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("ANTHROPIC_AGENT_ID", "agent-test")
	t.Setenv("ANTHROPIC_ENVIRONMENT_ID", "env-test")

	cfg := config.Load()
	svc, err := agent.NewService(cfg, agent.NewStore(testPool))
	if err != nil {
		t.Fatalf("agent service: %v", err)
	}
	return svc
}

func newAgentApp(t *testing.T, svc *agent.Service, userID, teamID, role string) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: apperrors.FiberHandler})
	api := app.Group("/api", stubMW(userID, teamID, role))
	noopLimiter := func(c *fiber.Ctx) error { return c.Next() }
	agent.NewHandler(svc).Mount(api, noopLimiter)
	return app
}

func doGET(t *testing.T, app *fiber.App, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func doDELETE(t *testing.T, app *fiber.App, path string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestHandler_ListScopes verifies ?scope=mine|all behavior:
// - members default to "mine" and cannot use scope=all
// - admins/owners default to "mine" but can pass scope=all to see everyone
func TestHandler_ListScopes(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	svc := newAgentService(t)
	store := svc.Store()
	owner, _, _ := freshUser(t)
	teamID := freshTeam(t, owner, "ScopeTest")

	tsvc := teams.NewService(teams.NewStore(testPool), 50)
	member, _, _ := freshUser(t)
	if err := tsvc.Store().AddMember(context.Background(), teamID, member, teams.RoleMember); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateSession(context.Background(), teamID, owner, "owner-1", nextAntID("scope-o1")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(context.Background(), teamID, member, "member-1", nextAntID("scope-m1")); err != nil {
		t.Fatal(err)
	}

	memberApp := newAgentApp(t, svc, member, teamID, string(teams.RoleMember))

	// Member default scope returns own session only.
	code, body := doGET(t, memberApp, "/api/agent/sessions")
	if code != 200 {
		t.Fatalf("member list: %d %s", code, body)
	}
	var listed struct {
		Sessions []agent.Session `json:"sessions"`
		Scope    string          `json:"scope"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Scope != "mine" || len(listed.Sessions) != 1 || listed.Sessions[0].UserID != member {
		t.Fatalf("member default: expected own session only, got %+v", listed)
	}

	// Member trying scope=all gets 403.
	code, _ = doGET(t, memberApp, "/api/agent/sessions?scope=all")
	if code != http.StatusForbidden {
		t.Fatalf("member scope=all: expected 403, got %d", code)
	}

	// Owner default also "mine".
	ownerApp := newAgentApp(t, svc, owner, teamID, string(teams.RoleOwner))
	code, body = doGET(t, ownerApp, "/api/agent/sessions")
	if code != 200 {
		t.Fatalf("owner list: %d %s", code, body)
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Scope != "mine" || len(listed.Sessions) != 1 {
		t.Fatalf("owner default: expected own session only, got %+v", listed)
	}

	// Owner with scope=all sees both.
	code, body = doGET(t, ownerApp, "/api/agent/sessions?scope=all")
	if code != 200 {
		t.Fatalf("owner scope=all: %d %s", code, body)
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Scope != "all" || len(listed.Sessions) != 2 {
		t.Fatalf("owner scope=all: expected 2 sessions, got %+v", listed)
	}
}

// TestHandler_GetCrossUser checks that members get 404 (not 403) when peeking
// at another member's session, while admins+ get 200.
func TestHandler_GetCrossUser(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	svc := newAgentService(t)
	store := svc.Store()
	owner, _, _ := freshUser(t)
	teamID := freshTeam(t, owner, "Peek")
	tsvc := teams.NewService(teams.NewStore(testPool), 50)
	memberA, _, _ := freshUser(t)
	memberB, _, _ := freshUser(t)
	for _, m := range []string{memberA, memberB} {
		if err := tsvc.Store().AddMember(context.Background(), teamID, m, teams.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := store.CreateSession(context.Background(), teamID, memberA, "A's chat", nextAntID("peek-a"))
	if err != nil {
		t.Fatal(err)
	}

	bApp := newAgentApp(t, svc, memberB, teamID, string(teams.RoleMember))
	code, _ := doGET(t, bApp, "/api/agent/sessions/"+sess.ID)
	if code != http.StatusNotFound {
		t.Fatalf("member peek: expected 404, got %d", code)
	}

	ownerApp := newAgentApp(t, svc, owner, teamID, string(teams.RoleOwner))
	code, body := doGET(t, ownerApp, "/api/agent/sessions/"+sess.ID)
	if code != 200 {
		t.Fatalf("owner peek: %d %s", code, body)
	}
}

// TestHandler_DeleteAuthorization verifies:
// - members can delete their own sessions
// - members cannot delete another member's session (404 first, since it acts
//   like read access — we don't reveal existence)
// - admins can delete any session in the team
func TestHandler_DeleteAuthorization(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	svc := newAgentService(t)
	store := svc.Store()
	owner, _, _ := freshUser(t)
	teamID := freshTeam(t, owner, "DelAuth")
	tsvc := teams.NewService(teams.NewStore(testPool), 50)
	mA, _, _ := freshUser(t)
	mB, _, _ := freshUser(t)
	for _, m := range []string{mA, mB} {
		if err := tsvc.Store().AddMember(context.Background(), teamID, m, teams.RoleMember); err != nil {
			t.Fatal(err)
		}
	}

	sessA, err := store.CreateSession(context.Background(), teamID, mA, "A1", nextAntID("delauth-a"))
	if err != nil {
		t.Fatal(err)
	}
	sessB, err := store.CreateSession(context.Background(), teamID, mB, "B1", nextAntID("delauth-b"))
	if err != nil {
		t.Fatal(err)
	}
	sessOwner, err := store.CreateSession(context.Background(), teamID, owner, "O1", nextAntID("delauth-o"))
	if err != nil {
		t.Fatal(err)
	}

	// Member A deletes own session: ok.
	aApp := newAgentApp(t, svc, mA, teamID, string(teams.RoleMember))
	if got := doDELETE(t, aApp, "/api/agent/sessions/"+sessA.ID); got != http.StatusNoContent {
		t.Fatalf("delete own: expected 204, got %d", got)
	}
	// Member A trying to delete B's session: 404 (read fails first).
	if got := doDELETE(t, aApp, "/api/agent/sessions/"+sessB.ID); got != http.StatusNotFound {
		t.Fatalf("delete other-member: expected 404, got %d", got)
	}
	// Owner deleting B's session: ok.
	ownerApp := newAgentApp(t, svc, owner, teamID, string(teams.RoleOwner))
	if got := doDELETE(t, ownerApp, "/api/agent/sessions/"+sessB.ID); got != http.StatusNoContent {
		t.Fatalf("owner delete other: expected 204, got %d", got)
	}
	// And owner deleting their own.
	if got := doDELETE(t, ownerApp, "/api/agent/sessions/"+sessOwner.ID); got != http.StatusNoContent {
		t.Fatalf("owner delete own: expected 204, got %d", got)
	}
}

// TestHandler_ScopeBadValue rejects unknown scope values with 400.
func TestHandler_ScopeBadValue(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	svc := newAgentService(t)
	owner, _, _ := freshUser(t)
	teamID := freshTeam(t, owner, "BadScope")
	app := newAgentApp(t, svc, owner, teamID, string(teams.RoleOwner))
	code, _ := doGET(t, app, "/api/agent/sessions?scope=banana")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad scope, got %d", code)
	}
}
