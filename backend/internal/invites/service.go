package invites

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

type Config struct {
	AppName     string
	AppURL      string
	FromName    string
	InviteTTL   time.Duration
	DefaultRole teams.Role
}

func (c Config) withDefaults() Config {
	if c.InviteTTL <= 0 {
		c.InviteTTL = 7 * 24 * time.Hour
	}
	if strings.TrimSpace(c.AppName) == "" {
		c.AppName = "Agent App"
	}
	if c.DefaultRole == "" {
		c.DefaultRole = teams.RoleMember
	}
	return c
}

type Service struct {
	cfg      Config
	teamsSvc *teams.Service
	authSvc  *auth.Service
	sender   EmailSender
	now      func() time.Time
}

func NewService(cfg Config, teamsSvc *teams.Service, authSvc *auth.Service, sender EmailSender) *Service {
	cfg = cfg.withDefaults()
	if sender == nil {
		sender = devLogger{}
	}
	return &Service{cfg: cfg, teamsSvc: teamsSvc, authSvc: authSvc, sender: sender, now: time.Now}
}

// Preview returns a public, no-auth view of the invite suitable for the
// invite landing page. Returns ErrInviteNotFound for unknown tokens and
// ErrInviteExpired/Revoked/AlreadyAccepted for terminal states.
type Preview struct {
	Team        teams.Team `json:"team"`
	Email       string     `json:"email"`
	Role        teams.Role `json:"role"`
	InvitedBy   string     `json:"invited_by_email"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RequiresLogin bool     `json:"requires_login"`
}

func (s *Service) Preview(ctx context.Context, token string) (Preview, error) {
	inv, err := s.teamsSvc.Store().GetInviteByToken(ctx, token)
	if err != nil {
		return Preview{}, err
	}
	if !inv.Active(s.now()) {
		switch {
		case inv.AcceptedAt != nil:
			return Preview{}, apperrors.ErrInviteAlreadyAccepted
		case inv.RevokedAt != nil:
			return Preview{}, apperrors.ErrInviteRevoked
		default:
			return Preview{}, apperrors.ErrInviteExpired
		}
	}
	team, err := s.teamsSvc.Store().GetTeam(ctx, inv.TeamID)
	if err != nil {
		return Preview{}, err
	}
	inviter, err := s.authSvc.GetUser(ctx, inv.InvitedBy)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return Preview{}, err
	}
	return Preview{
		Team:          team,
		Email:         inv.Email,
		Role:          inv.Role,
		InvitedBy:     inviter.Email,
		ExpiresAt:     inv.ExpiresAt,
		RequiresLogin: true,
	}, nil
}

// CreateAndSend mints an invite for the given email + role and dispatches the
// email. Caller (handler) is expected to have asserted admin+ on the team.
func (s *Service) CreateAndSend(ctx context.Context, teamID, inviterID, email string, role teams.Role) (teams.Invite, error) {
	clean, err := normalizeEmail(email)
	if err != nil {
		return teams.Invite{}, err
	}
	if !role.Valid() || role == teams.RoleOwner {
		return teams.Invite{}, apperrors.New(400, "invalid role; must be admin or member")
	}

	inviter, err := s.authSvc.GetUser(ctx, inviterID)
	if err != nil {
		return teams.Invite{}, err
	}

	if err := s.teamsSvc.EnforceSeatLimit(ctx, teamID, 1); err != nil {
		return teams.Invite{}, err
	}

	tok, err := teams.NewInviteToken()
	if err != nil {
		return teams.Invite{}, err
	}

	expiresAt := s.now().Add(s.cfg.InviteTTL)
	inv, err := s.teamsSvc.Store().CreateInvite(ctx, teamID, inviterID, clean, role, tok, expiresAt)
	if err != nil {
		return teams.Invite{}, err
	}

	team, err := s.teamsSvc.Store().GetTeam(ctx, teamID)
	if err != nil {
		return teams.Invite{}, err
	}

	if err := s.send(ctx, team, inv, inviter); err != nil {
		// Rolling back the row prevents a "stuck pending invite" that you can
		// never resend because the unique-pending-email index blocks it.
		_ = s.teamsSvc.Store().MarkInviteRevoked(ctx, inv.ID, s.now())
		return teams.Invite{}, apperrors.New(502, "failed to dispatch invite email; please try again")
	}
	return inv, nil
}

// Resend regenerates the email for an existing pending invite. The invite ID
// (not token) is referenced so admins can re-send without exposing tokens in
// listings. The TTL is *not* extended.
func (s *Service) Resend(ctx context.Context, teamID, actorID, inviteID string) (teams.Invite, error) {
	inv, err := s.teamsSvc.Store().GetInvite(ctx, inviteID)
	if err != nil {
		return teams.Invite{}, err
	}
	if inv.TeamID != teamID {
		return teams.Invite{}, apperrors.ErrInviteNotFound
	}
	if !inv.Active(s.now()) {
		return teams.Invite{}, apperrors.ErrInviteExpired
	}
	team, err := s.teamsSvc.Store().GetTeam(ctx, teamID)
	if err != nil {
		return teams.Invite{}, err
	}
	inviter, err := s.authSvc.GetUser(ctx, actorID)
	if err != nil {
		return teams.Invite{}, err
	}
	if err := s.send(ctx, team, inv, inviter); err != nil {
		return teams.Invite{}, apperrors.New(502, "failed to dispatch invite email; please try again")
	}
	return inv, nil
}

// AcceptByToken consumes an invite for the authenticated user. Email must
// match the invite (case-insensitive) so links can't be forwarded to a friend.
func (s *Service) AcceptByToken(ctx context.Context, userID, token string) (teams.Team, teams.Role, error) {
	user, err := s.authSvc.GetUser(ctx, userID)
	if err != nil {
		return teams.Team{}, "", err
	}
	inv, err := s.teamsSvc.Store().GetInviteByToken(ctx, token)
	if err != nil {
		return teams.Team{}, "", err
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(inv.Email)) {
		return teams.Team{}, "", apperrors.ErrEmailMismatch
	}
	consumed, err := s.teamsSvc.Store().ConsumeInvite(ctx, token, userID, s.now())
	if err != nil {
		return teams.Team{}, "", err
	}
	team, err := s.teamsSvc.Store().GetTeam(ctx, consumed.TeamID)
	if err != nil {
		return teams.Team{}, "", err
	}
	return team, consumed.Role, nil
}

// ListActive returns all non-accepted, non-revoked invites for the team.
// Caller should have asserted membership.
func (s *Service) ListActive(ctx context.Context, teamID string) ([]teams.Invite, error) {
	return s.teamsSvc.Store().ListInvitesForTeam(ctx, teamID)
}

func (s *Service) Revoke(ctx context.Context, teamID, inviteID string) error {
	inv, err := s.teamsSvc.Store().GetInvite(ctx, inviteID)
	if err != nil {
		return err
	}
	if inv.TeamID != teamID {
		return apperrors.ErrInviteNotFound
	}
	return s.teamsSvc.Store().MarkInviteRevoked(ctx, inviteID, s.now())
}

func (s *Service) send(ctx context.Context, team teams.Team, inv teams.Invite, inviter auth.User) error {
	url := strings.TrimRight(s.cfg.AppURL, "/") + "/invites/accept?token=" + inv.Token
	subject, body := renderInviteEmail(EmailRequest{
		AppName:       s.cfg.AppName,
		FromName:      s.cfg.FromName,
		InviterEmail:  inviter.Email,
		InviterName:   inviter.Name,
		TeamName:      team.Name,
		Role:          string(inv.Role),
		AcceptURL:     url,
		ExpiresInDays: int(s.cfg.InviteTTL / (24 * time.Hour)),
	})
	return s.sender.Send(ctx, inv.Email, subject, body)
}

func normalizeEmail(in string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(in))
	if err != nil {
		return "", apperrors.New(400, "invalid email address")
	}
	return strings.ToLower(addr.Address), nil
}
