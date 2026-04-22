package teams

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// Service exposes team + membership operations with role enforcement.
type Service struct {
	store          *Store
	defaultMaxSeat int
}

func NewService(store *Store, defaultMaxSeats int) *Service {
	if defaultMaxSeats <= 0 {
		defaultMaxSeats = 25
	}
	return &Service{store: store, defaultMaxSeat: defaultMaxSeats}
}

func (s *Service) Store() *Store { return s.store }

// EnsurePersonalTeam returns the user's personal team, creating it on first
// call. Safe to invoke from auth on every login — it short-circuits if a
// personal team already exists.
func (s *Service) EnsurePersonalTeam(ctx context.Context, userID, displayName, email string) (Team, error) {
	if t, err := s.store.FindPersonalTeam(ctx, userID); err == nil {
		return t, nil
	} else if !errors.Is(err, apperrors.ErrTeamNotFound) {
		return Team{}, err
	}

	name := personalTeamName(displayName, email)
	team, err := s.store.CreateTeamWithOwner(ctx, userID, name, true, s.defaultMaxSeat)
	// Tolerate races: another concurrent call may have inserted the row first.
	if errors.Is(err, apperrors.ErrPersonalTeamExists) {
		return s.store.FindPersonalTeam(ctx, userID)
	}
	return team, err
}

// Create makes a new (non-personal) team owned by ownerID.
func (s *Service) Create(ctx context.Context, ownerID, name string) (Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Team{}, apperrors.New(400, "team name is required")
	}
	if len(name) > 80 {
		return Team{}, apperrors.New(400, "team name is too long (max 80 chars)")
	}
	return s.store.CreateTeamWithOwner(ctx, ownerID, name, false, s.defaultMaxSeat)
}

func (s *Service) Get(ctx context.Context, teamID string) (Team, error) {
	return s.store.GetTeam(ctx, teamID)
}

func (s *Service) ListForUser(ctx context.Context, userID string) ([]Membership, error) {
	return s.store.ListTeamsForUser(ctx, userID)
}

// ResolveActive selects the membership the request operates on. If a header
// team ID is provided, it must be one the user belongs to. Otherwise the
// user's personal team is returned (created on demand for safety).
func (s *Service) ResolveActive(ctx context.Context, userID, requestedTeamID, displayName, email string) (Membership, error) {
	requested := strings.TrimSpace(requestedTeamID)
	if requested != "" {
		return s.store.GetMembership(ctx, requested, userID)
	}
	team, err := s.EnsurePersonalTeam(ctx, userID, displayName, email)
	if err != nil {
		return Membership{}, err
	}
	return Membership{Team: team, Role: RoleOwner}, nil
}

func (s *Service) UpdateName(ctx context.Context, actorID, teamID, name string) (Team, error) {
	actor, err := s.store.GetMembership(ctx, teamID, actorID)
	if err != nil {
		return Team{}, err
	}
	if !actor.Role.AtLeast(RoleAdmin) {
		return Team{}, apperrors.ErrInsufficientRole
	}
	if strings.TrimSpace(name) == "" {
		return Team{}, apperrors.New(400, "team name is required")
	}
	return s.store.UpdateTeamName(ctx, teamID, name)
}

func (s *Service) Delete(ctx context.Context, actorID, teamID string) error {
	actor, err := s.store.GetMembership(ctx, teamID, actorID)
	if err != nil {
		return err
	}
	if actor.Role != RoleOwner {
		return apperrors.ErrInsufficientRole
	}
	if actor.Team.IsPersonal {
		return apperrors.ErrCannotDeletePersonal
	}
	return s.store.DeleteTeam(ctx, teamID)
}

func (s *Service) TransferOwnership(ctx context.Context, actorID, teamID, toUserID string) error {
	if actorID == toUserID {
		return apperrors.New(400, "cannot transfer ownership to yourself")
	}
	actor, err := s.store.GetMembership(ctx, teamID, actorID)
	if err != nil {
		return err
	}
	if actor.Role != RoleOwner {
		return apperrors.ErrInsufficientRole
	}
	if actor.Team.IsPersonal {
		return apperrors.New(400, "cannot transfer ownership of a personal team")
	}
	if _, err := s.store.GetMembership(ctx, teamID, toUserID); err != nil {
		return err
	}
	return s.store.TransferOwnership(ctx, teamID, actorID, toUserID)
}

func (s *Service) ListMembers(ctx context.Context, actorID, teamID string) ([]Member, error) {
	if _, err := s.store.GetMembership(ctx, teamID, actorID); err != nil {
		return nil, err
	}
	return s.store.ListMembers(ctx, teamID)
}

// ChangeRole permits admin+ to flip another member between admin/member.
// The owner is untouchable via this path; ownership transfers are explicit.
func (s *Service) ChangeRole(ctx context.Context, actorID, teamID, targetID string, newRole Role) error {
	if !newRole.Valid() {
		return apperrors.New(400, "invalid role")
	}
	if newRole == RoleOwner {
		return apperrors.New(400, "use transfer-ownership to assign the owner role")
	}
	actor, err := s.store.GetMembership(ctx, teamID, actorID)
	if err != nil {
		return err
	}
	if !actor.Role.AtLeast(RoleAdmin) {
		return apperrors.ErrInsufficientRole
	}
	target, err := s.store.GetMembership(ctx, teamID, targetID)
	if err != nil {
		return err
	}
	if target.Role == RoleOwner {
		return apperrors.New(400, "cannot change the owner's role")
	}
	if !canActOn(actor.Role, target.Role) {
		return apperrors.ErrInsufficientRole
	}
	return s.store.UpdateMemberRole(ctx, teamID, targetID, newRole)
}

func (s *Service) RemoveMember(ctx context.Context, actorID, teamID, targetID string) error {
	if actorID == targetID {
		return apperrors.New(400, "use leave-team to remove yourself")
	}
	actor, err := s.store.GetMembership(ctx, teamID, actorID)
	if err != nil {
		return err
	}
	if !actor.Role.AtLeast(RoleAdmin) {
		return apperrors.ErrInsufficientRole
	}
	target, err := s.store.GetMembership(ctx, teamID, targetID)
	if err != nil {
		return err
	}
	if target.Role == RoleOwner {
		return apperrors.ErrCannotRemoveOwner
	}
	if !canActOn(actor.Role, target.Role) {
		return apperrors.ErrInsufficientRole
	}
	return s.store.RemoveMember(ctx, teamID, targetID)
}

// Leave removes the caller from a team. Forbidden when the user is the sole
// owner of a non-personal team, or when the team is the user's personal team.
func (s *Service) Leave(ctx context.Context, userID, teamID string) error {
	membership, err := s.store.GetMembership(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if membership.Team.IsPersonal {
		return apperrors.ErrCannotLeavePersonal
	}
	if membership.Role == RoleOwner {
		return apperrors.New(400, "owner must transfer ownership before leaving")
	}
	return s.store.RemoveMember(ctx, teamID, userID)
}

// canActOn enforces the rule that admins cannot manage other admins or
// the owner. Owners can manage anyone except themselves through these paths.
func canActOn(actor, target Role) bool {
	switch actor {
	case RoleOwner:
		return true
	case RoleAdmin:
		return target == RoleMember
	default:
		return false
	}
}

// EnforceSeatLimit returns ErrSeatLimitReached if pendingNew would push the
// team over its max_seats setting. pendingNew accounts for both current
// members and active invites.
func (s *Service) EnforceSeatLimit(ctx context.Context, teamID string, pendingInvites int) error {
	t, err := s.store.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	count, err := s.store.CountMembers(ctx, teamID)
	if err != nil {
		return err
	}
	if count+pendingInvites >= t.MaxSeats {
		return apperrors.ErrSeatLimitReached
	}
	return nil
}

// personalTeamName picks a friendly default for the auto-created team.
func personalTeamName(displayName, email string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name + "'s Workspace"
	}
	if local := strings.SplitN(strings.TrimSpace(email), "@", 2)[0]; local != "" {
		return local + "'s Workspace"
	}
	return "Personal Workspace"
}

// SuggestUpdatedAt is a tiny helper for tests so we don't fight pgx mocks.
func SuggestUpdatedAt() time.Time { return time.Now() }
