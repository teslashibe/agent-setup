package main

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	magiclink "github.com/teslashibe/magiclink-auth-go"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/invites"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// sendMagicLinkHandler accepts {email, invite_token?}, validates the optional
// invite_token preview, hands the invite_token to the codeStore so it lands
// on the auth_codes row, then calls magicSvc.Send.
func sendMagicLinkHandler(magicSvc *magiclink.Service, codeStore *codeStoreAdapter, invitesSvc *invites.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Email       string `json:"email"`
			InviteToken string `json:"invite_token"`
		}
		if err := c.BodyParser(&req); err != nil {
			return apperrors.New(fiber.StatusBadRequest, "invalid request")
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email == "" {
			return apperrors.New(fiber.StatusBadRequest, "email is required")
		}

		inviteTok := strings.TrimSpace(req.InviteToken)
		if inviteTok != "" && invitesSvc != nil {
			preview, err := invitesSvc.Preview(c.UserContext(), inviteTok)
			if err != nil {
				return err
			}
			if !strings.EqualFold(preview.Email, email) {
				return apperrors.ErrEmailMismatch
			}
			codeStore.SetPendingInvite(email, inviteTok)
		} else {
			// Clear any stale pending invite for this email so a regular
			// re-send doesn't accidentally consume an old token.
			codeStore.SetPendingInvite(email, "")
		}

		if err := magicSvc.Send(c.UserContext(), email); err != nil {
			return c.Status(magiclink.HTTPStatus(err)).JSON(fiber.Map{
				"error": magiclink.PublicError(err),
			})
		}
		return c.JSON(fiber.Map{"status": "sent"})
	}
}

// verifyCodeHandler verifies an OTP, then if the underlying auth_codes row
// recorded an invite_token, accepts it before responding. The accepted team
// is included in the response under "invite" so the client can switch to it.
func verifyCodeHandler(magicSvc *magiclink.Service, codeStore *codeStoreAdapter, invitesSvc *invites.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := c.BodyParser(&req); err != nil {
			return apperrors.New(fiber.StatusBadRequest, "invalid request")
		}
		result, err := magicSvc.VerifyCode(c.UserContext(), req.Email, req.Code)
		if err != nil {
			return c.Status(magiclink.HTTPStatus(err)).JSON(fiber.Map{
				"error": magiclink.PublicError(err),
			})
		}

		body := fiber.Map{
			"token":   result.JWT,
			"user_id": result.UserID,
			"email":   result.Email,
			"name":    result.DisplayName,
		}

		if invitesSvc != nil {
			if inviteTok, lookupErr := codeStore.LookupInviteByEmail(c.UserContext(), result.Email); lookupErr == nil && inviteTok != "" {
				if team, role, acceptErr := invitesSvc.AcceptByToken(c.UserContext(), result.UserID, inviteTok); acceptErr == nil {
					body["invite"] = fiber.Map{
						"team": team,
						"role": role,
					}
				} else if !isExpectedAcceptError(acceptErr) {
					log.Printf("invite auto-accept failed for user=%s: %v", result.UserID, acceptErr)
					body["invite_error"] = errMessage(acceptErr)
				} else {
					body["invite_error"] = errMessage(acceptErr)
				}
			}
		}

		return c.JSON(body)
	}
}

// verifyLinkHandler handles GET /auth/verify?token=<magic-link>. Verifies the
// magic link, then accepts any attached invite, then renders the success page
// (which deep-links into the mobile app with the JWT).
func verifyLinkHandler(magicSvc *magiclink.Service, codeStore *codeStoreAdapter, invitesSvc *invites.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := strings.TrimSpace(c.Query("token"))
		if token == "" {
			return c.Status(fiber.StatusBadRequest).SendString("missing token")
		}

		// Look up the invite_token before verify (verify marks the row used).
		inviteTok, _ := codeStore.LookupInviteByToken(c.UserContext(), token)

		result, err := magicSvc.VerifyToken(c.UserContext(), token)
		if err != nil {
			return c.Status(magiclink.HTTPStatus(err)).SendString(magiclink.PublicError(err))
		}

		if inviteTok != "" && invitesSvc != nil {
			if _, _, acceptErr := invitesSvc.AcceptByToken(c.UserContext(), result.UserID, inviteTok); acceptErr != nil {
				log.Printf("invite auto-accept (link) failed for user=%s: %v", result.UserID, acceptErr)
			}
		}

		html, err := magicSvc.SuccessPageHTML(result)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(html)
	}
}

// isExpectedAcceptError returns true for invite-accept errors a user might
// reasonably encounter (already accepted, expired, email mismatch). They get
// surfaced via "invite_error" without spamming logs.
func isExpectedAcceptError(err error) bool {
	switch {
	case errors.Is(err, apperrors.ErrInviteAlreadyAccepted),
		errors.Is(err, apperrors.ErrInviteExpired),
		errors.Is(err, apperrors.ErrInviteRevoked),
		errors.Is(err, apperrors.ErrInviteNotFound),
		errors.Is(err, apperrors.ErrEmailMismatch):
		return true
	}
	return false
}

func errMessage(err error) string {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return err.Error()
}

// helper kept for lint: ensures context import is used even if no other
// identifier references it.
var _ = context.Background
var _ = teams.RoleMember
