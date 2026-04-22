import { request } from "@/services/api";

export type TeamRole = "owner" | "admin" | "member";

export type Team = {
  id: string;
  name: string;
  slug: string;
  is_personal: boolean;
  max_seats: number;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type Membership = {
  team: Team;
  role: TeamRole;
};

export type TeamMember = {
  team_id: string;
  user_id: string;
  email: string;
  name: string;
  role: TeamRole;
  joined_at: string;
};

export type Invite = {
  id: string;
  team_id: string;
  email: string;
  role: TeamRole;
  token?: string;
  invited_by: string;
  expires_at: string;
  accepted_at?: string;
  revoked_at?: string;
  created_at: string;
};

// roleAtLeast mirrors backend Role.AtLeast so we can gate UI on role tier.
const ROLE_RANK: Record<TeamRole, number> = { member: 0, admin: 1, owner: 2 };
export function roleAtLeast(role: TeamRole | undefined, min: TeamRole): boolean {
  if (!role) return false;
  return ROLE_RANK[role] >= ROLE_RANK[min];
}

// listMyTeams + createTeam don't depend on an active team; they operate on the
// caller identity. Skip X-Team-ID so the request remains valid even when no
// team is selected yet (e.g. immediately after login).
export async function listMyTeams(): Promise<Membership[]> {
  const res = await request<{ teams: Membership[] }>("/api/teams/", { skipTeamHeader: true });
  return res.teams ?? [];
}

export async function createTeam(name: string): Promise<Membership> {
  return request<Membership>("/api/teams/", {
    method: "POST",
    body: JSON.stringify({ name }),
    skipTeamHeader: true,
  });
}

export async function getTeam(teamID: string): Promise<Membership> {
  return request<Membership>(`/api/teams/${teamID}/`);
}

export async function updateTeamName(teamID: string, name: string): Promise<Team> {
  const res = await request<{ team: Team }>(`/api/teams/${teamID}/`, {
    method: "PATCH",
    body: JSON.stringify({ name }),
  });
  return res.team;
}

export async function deleteTeam(teamID: string): Promise<void> {
  await request<void>(`/api/teams/${teamID}/`, { method: "DELETE" });
}

export async function listMembers(teamID: string): Promise<TeamMember[]> {
  const res = await request<{ members: TeamMember[] }>(`/api/teams/${teamID}/members`);
  return res.members ?? [];
}

export async function updateMemberRole(
  teamID: string,
  userID: string,
  role: TeamRole,
): Promise<void> {
  await request<void>(`/api/teams/${teamID}/members/${userID}`, {
    method: "PATCH",
    body: JSON.stringify({ role }),
  });
}

export async function removeMember(teamID: string, userID: string): Promise<void> {
  await request<void>(`/api/teams/${teamID}/members/${userID}`, { method: "DELETE" });
}

export async function leaveTeam(teamID: string): Promise<void> {
  await request<void>(`/api/teams/${teamID}/members/me`, { method: "DELETE" });
}

export async function transferOwnership(teamID: string, toUserID: string): Promise<void> {
  await request<void>(`/api/teams/${teamID}/transfer-ownership`, {
    method: "POST",
    body: JSON.stringify({ to_user_id: toUserID }),
  });
}

// ----- Invites --------------------------------------------------------------

export async function listInvites(teamID: string): Promise<Invite[]> {
  const res = await request<{ invites: Invite[] }>(`/api/teams/${teamID}/invites`);
  return res.invites ?? [];
}

export async function createInvite(
  teamID: string,
  email: string,
  role: Exclude<TeamRole, "owner">,
): Promise<Invite> {
  return request<Invite>(`/api/teams/${teamID}/invites`, {
    method: "POST",
    body: JSON.stringify({ email, role }),
  });
}

export async function resendInvite(teamID: string, inviteID: string): Promise<Invite> {
  return request<Invite>(`/api/teams/${teamID}/invites/${inviteID}/resend`, { method: "POST" });
}

export async function revokeInvite(teamID: string, inviteID: string): Promise<void> {
  await request<void>(`/api/teams/${teamID}/invites/${inviteID}`, { method: "DELETE" });
}

// previewInvite is unauthenticated so the invite landing screen can render the
// team name/role before the recipient signs in. Hits the spec-shaped path
// `GET /api/invites/:token`.
export async function previewInvite(token: string): Promise<{
  team: { id: string; name: string };
  email: string;
  role: TeamRole;
  expires_at: string;
}> {
  return request(`/api/invites/${encodeURIComponent(token)}`, {
    method: "GET",
    auth: false,
  });
}

// acceptInvite hits the spec-shaped path `POST /api/invites/:token/accept`.
// The auth check + email-match check happens server-side.
export async function acceptInvite(token: string): Promise<{ team: Team; role: TeamRole }> {
  return request(`/api/invites/${encodeURIComponent(token)}/accept`, {
    method: "POST",
  });
}
