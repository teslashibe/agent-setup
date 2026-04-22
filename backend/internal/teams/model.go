package teams

import "time"

// Role identifies a member's permissions within a team.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Valid reports whether r is one of the three known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	}
	return false
}

// AtLeast returns true if r is at least as privileged as min.
// Ordering: owner > admin > member.
func (r Role) AtLeast(min Role) bool {
	return roleRank(r) >= roleRank(min)
}

func roleRank(r Role) int {
	switch r {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	}
	return 0
}

type Team struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	IsPersonal bool      `json:"is_personal"`
	MaxSeats   int       `json:"max_seats"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Member is the (team, user, role) tuple plus the user's contact info.
type Member struct {
	TeamID   string    `json:"team_id"`
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     Role      `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Membership pairs a Team with the caller's role inside it.
type Membership struct {
	Team Team `json:"team"`
	Role Role `json:"role"`
}

type Invite struct {
	ID         string     `json:"id"`
	TeamID     string     `json:"team_id"`
	Email      string     `json:"email"`
	Role       Role       `json:"role"`
	Token      string     `json:"token,omitempty"`
	InvitedBy  string     `json:"invited_by"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Active reports whether an invite can still be accepted at t.
func (i Invite) Active(t time.Time) bool {
	if i.AcceptedAt != nil || i.RevokedAt != nil {
		return false
	}
	return t.Before(i.ExpiresAt)
}
