import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

import {
  getTeam,
  type Invite,
  listInvites,
  listMembers,
  type Membership,
  type TeamMember,
} from "@/services/teams";
import { useTeams, type TeamAction } from "@/providers/TeamsProvider";

// TeamPage is the shared per-team data fetched once at the [id]/_layout level
// so the overview/members/invites tabs don't all refetch on navigation.
//
// canIn is wired through to the central TeamsProvider table — children call
// can("members.changeRole") instead of reading raw role tiers.
type TeamPageValue = {
  teamID: string;
  membership: Membership | null;
  members: TeamMember[];
  invites: Invite[];
  loading: boolean;
  refresh: () => Promise<void>;
  can: (action: TeamAction) => boolean;
};

const TeamPageContext = createContext<TeamPageValue | null>(null);

export function TeamPageProvider({
  teamID,
  children,
}: {
  teamID: string;
  children: React.ReactNode;
}) {
  const { canIn } = useTeams();

  const [membership, setMembership] = useState<Membership | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    if (!teamID) return;
    const [m, mems] = await Promise.all([getTeam(teamID), listMembers(teamID)]);
    setMembership(m);
    setMembers(
      [...mems].sort(
        (a, b) =>
          // owner < admin < member, then alpha by email so the list is stable.
          roleOrder[a.role] - roleOrder[b.role] || a.email.localeCompare(b.email),
      ),
    );
    if (canIn(m, "invites.list")) {
      const inv = await listInvites(teamID);
      setInvites(inv);
    } else {
      setInvites([]);
    }
  }, [teamID, canIn]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    refresh()
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [refresh]);

  const can = useCallback(
    (action: TeamAction) => canIn(membership, action),
    [canIn, membership],
  );

  const value = useMemo<TeamPageValue>(
    () => ({ teamID, membership, members, invites, loading, refresh, can }),
    [teamID, membership, members, invites, loading, refresh, can],
  );

  return <TeamPageContext.Provider value={value}>{children}</TeamPageContext.Provider>;
}

export function useTeamPage() {
  const ctx = useContext(TeamPageContext);
  if (!ctx) {
    throw new Error("useTeamPage must be used inside a TeamPageProvider (teams/[id]/_layout)");
  }
  return ctx;
}

const roleOrder = { owner: 0, admin: 1, member: 2 } as const;
