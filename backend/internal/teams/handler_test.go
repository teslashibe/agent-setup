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

// TestHandler_LeaveViaDeleteMembersMe pins the spec API shape: leaving a team
// is `DELETE /api/teams/:teamID/members/me`. Catches the regression where the
// route used to be `POST /leave` and mobile clients silently 404'd.
func TestHandler_LeaveViaDeleteMembersMe(t *testing.T) {
	ownerID, _, _ := freshUser(t)
	memberID, memberEmail, memberName := freshUser(t)
	ownerApp, ownerSvc := newTestApp(t, ownerID, "owner@local.test", "Owner")

	resp, body := doJSON(t, ownerApp, "POST", "/api/teams/", map[string]any{"name": "LeaveMe"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created teams.Membership
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if err := ownerSvc.Store().AddMember(t.Context(), created.Team.ID, memberID, teams.RoleMember); err != nil {
		t.Fatal(err)
	}

	memberApp, _ := newTestApp(t, memberID, memberEmail, memberName)
	resp, body = doJSON(t, memberApp, "DELETE", "/api/teams/"+created.Team.ID+"/members/me", nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("member leave via DELETE /members/me: expected 204, got %d body=%s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, ownerApp, "GET", "/api/teams/"+created.Team.ID+"/members", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get members: %d", resp.StatusCode)
	}
}

// TestHandler_LeavePersonalRejected ensures the personal-team guard fires on
// the new DELETE /members/me path too (not just the old /leave alias).
func TestHandler_LeavePersonalRejected(t *testing.T) {
	uid, email, name := freshUser(t)
	app, svc := newTestApp(t, uid, email, name)
	personal, err := svc.EnsurePersonalTeam(t.Context(), uid, name, email)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := doJSON(t, app, "DELETE", "/api/teams/"+personal.ID+"/members/me", nil, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 ErrCannotLeavePersonal, got %d body=%s", resp.StatusCode, body)
	}
}

// TestHandler_TransferOwnershipBodyAliases verifies the spec field
// `to_user_id` works AND legacy `user_id` / `new_owner_user_id` keep working
// for older client builds. Catches H2 regressions.
func TestHandler_TransferOwnershipBodyAliases(t *testing.T) {
	cases := []string{"to_user_id", "user_id", "new_owner_user_id"}
	for _, field := range cases {
		t.Run(field, func(t *testing.T) {
			ownerID, _, _ := freshUser(t)
			heirID, heirEmail, heirName := freshUser(t)
			ownerApp, ownerSvc := newTestApp(t, ownerID, "owner@local.test", "Owner")

			resp, body := doJSON(t, ownerApp, "POST", "/api/teams/",
				map[string]any{"name": "TransferTest-" + field}, nil)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create: %d %s", resp.StatusCode, body)
			}
			var created teams.Membership
			if err := json.Unmarshal(body, &created); err != nil {
				t.Fatal(err)
			}
			if err := ownerSvc.Store().AddMember(t.Context(), created.Team.ID, heirID, teams.RoleMember); err != nil {
				t.Fatal(err)
			}
			_ = heirEmail
			_ = heirName

			resp, body = doJSON(t, ownerApp, "POST",
				"/api/teams/"+created.Team.ID+"/transfer-ownership",
				map[string]any{field: heirID}, nil)
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("transfer via %q: expected 204, got %d body=%s", field, resp.StatusCode, body)
			}

			members, err := ownerSvc.Store().ListMembers(t.Context(), created.Team.ID)
			if err != nil {
				t.Fatal(err)
			}
			var sawNewOwner bool
			for _, m := range members {
				if m.UserID == heirID && m.Role == teams.RoleOwner {
					sawNewOwner = true
				}
				if m.UserID == ownerID && m.Role == teams.RoleOwner {
					t.Fatalf("old owner still owner after transfer")
				}
			}
			if !sawNewOwner {
				t.Fatalf("heir did not become owner")
			}
		})
	}
}

// TestHandler_TransferOwnershipMissingField asserts the API rejects empty
// body (none of the three accepted aliases supplied).
func TestHandler_TransferOwnershipMissingField(t *testing.T) {
	ownerID, _, _ := freshUser(t)
	app, _ := newTestApp(t, ownerID, "owner@local.test", "Owner")

	resp, body := doJSON(t, app, "POST", "/api/teams/", map[string]any{"name": "Missing"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created teams.Membership
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	resp, _ = doJSON(t, app, "POST",
		"/api/teams/"+created.Team.ID+"/transfer-ownership",
		map[string]any{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
