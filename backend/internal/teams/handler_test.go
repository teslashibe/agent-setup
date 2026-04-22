package teams_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// fakeAuth installs synthetic user_id locals so we can exercise the handler
// without booting the real magic-link middleware.
func fakeAuth(userID, email, name string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apperrors.SetUserID(c, userID)
		apperrors.SetUserEmail(c, email)
		apperrors.SetUserDisplayName(c, name)
		return c.Next()
	}
}

func newTestApp(t *testing.T, userID, email, name string) (*fiber.App, *teams.Service) {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	svc := teams.NewService(teams.NewStore(testPool), 10)
	mw := teams.NewMiddleware(svc)

	if _, err := svc.EnsurePersonalTeam(t.Context(), userID, name, email); err != nil {
		t.Fatalf("bootstrap personal team: %v", err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: apperrors.FiberHandler})
	api := app.Group("/api", fakeAuth(userID, email, name))
	teams.NewHandler(svc, mw).Mount(api)
	return app, svc
}

func doJSON(t *testing.T, app *fiber.App, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody
}

func TestHandler_CreateAndList(t *testing.T) {
	uid, email, name := freshUser(t)
	app, _ := newTestApp(t, uid, email, name)

	resp, body := doJSON(t, app, "POST", "/api/teams/", map[string]any{"name": "Demo Co"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, app, "GET", "/api/teams/", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var listed struct {
		Teams []teams.Membership `json:"teams"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Teams) < 2 {
		t.Fatalf("expected personal + new team, got %d", len(listed.Teams))
	}
	var sawPersonal, sawDemo bool
	for _, m := range listed.Teams {
		if m.Team.IsPersonal {
			sawPersonal = true
		}
		if m.Team.Name == "Demo Co" && m.Role == teams.RoleOwner {
			sawDemo = true
		}
	}
	if !sawPersonal || !sawDemo {
		t.Fatalf("expected personal+Demo Co, got %+v", listed.Teams)
	}
}

func TestHandler_NonMemberCannotAccess(t *testing.T) {
	ownerID, _, _ := freshUser(t)
	intruderID, intruderEmail, intruderName := freshUser(t)

	ownerApp, ownerSvc := newTestApp(t, ownerID, "owner@test.local", "Owner")
	resp, body := doJSON(t, ownerApp, "POST", "/api/teams/", map[string]any{"name": "Private"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created teams.Membership
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	intruderApp, _ := newTestApp(t, intruderID, intruderEmail, intruderName)
	_ = ownerSvc

	resp, _ = doJSON(t, intruderApp, "GET", "/api/teams/"+created.Team.ID+"/", nil, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("intruder GET: expected 403, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, intruderApp, "GET", "/api/teams/"+created.Team.ID+"/members", nil, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("intruder members: expected 403, got %d", resp.StatusCode)
	}
}

func TestHandler_AdminCannotDeleteTeam(t *testing.T) {
	ownerID, _, _ := freshUser(t)
	adminID, adminEmail, adminName := freshUser(t)
	ownerApp, ownerSvc := newTestApp(t, ownerID, "o@test.local", "O")

	resp, body := doJSON(t, ownerApp, "POST", "/api/teams/", map[string]any{"name": "DelTest"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created teams.Membership
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if err := ownerSvc.Store().AddMember(t.Context(), created.Team.ID, adminID, teams.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	adminApp, _ := newTestApp(t, adminID, adminEmail, adminName)
	resp, _ = doJSON(t, adminApp, "DELETE", "/api/teams/"+created.Team.ID+"/", nil, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin delete: expected 403, got %d", resp.StatusCode)
	}

	resp, _ = doJSON(t, ownerApp, "DELETE", "/api/teams/"+created.Team.ID+"/", nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("owner delete: expected 204, got %d", resp.StatusCode)
	}
}

func TestHandler_HeaderResolutionDefaultsToPersonal(t *testing.T) {
	uid, email, name := freshUser(t)
	app, svc := newTestApp(t, uid, email, name)

	if _, err := svc.EnsurePersonalTeam(t.Context(), uid, name, email); err != nil {
		t.Fatal(err)
	}

	listResp, body := doJSON(t, app, "GET", "/api/teams/", nil, nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", listResp.StatusCode, body)
	}
}
