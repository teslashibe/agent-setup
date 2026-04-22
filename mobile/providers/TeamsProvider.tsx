import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";

import {
  acceptInvite as acceptInviteAPI,
  type Membership,
  listMyTeams,
  roleAtLeast,
  type TeamRole,
} from "@/services/teams";
import { setActiveTeamProvider } from "@/services/api";
import { useAuthSession } from "@/providers/AuthSessionProvider";

// Action is the closed set of UI gates the mobile client knows how to ask
// about. Mirrors the server's permission gates in teams.Service.canActOn /
// invites.Service.* — keeping them named (not raw role checks) means the
// rule lives in one place and the views just declare intent.
//
// Pre-checks like this are advisory only — the server is still the source
// of truth. A stale role here can't grant anything; the server will 403.
export type TeamAction =
  | "team.update"               // PATCH /api/teams/:teamID            (admin+)
  | "team.delete"               // DELETE /api/teams/:teamID           (owner)
  | "team.transferOwnership"    // POST   /transfer-ownership          (owner)
  | "members.list"              // any member can see the roster
  | "members.changeRole"        // PATCH  /members/:userID             (admin+)
  | "members.remove"            // DELETE /members/:userID             (admin+)
  | "members.leave"             // DELETE /members/me                  (any non-owner non-personal)
  | "invites.list"              // any member can see pending invites
  | "invites.create"            // POST   /invites                     (admin+)
  | "invites.resend"            // POST   /invites/:id/resend          (admin+)
  | "invites.revoke"            // DELETE /invites/:id                 (admin+)
  | "agent.viewAllSessions";    // GET    /api/sessions?scope=all      (admin+)

type TeamsContextValue = {
  isLoading: boolean;
  memberships: Membership[];
  active: Membership | null;
  setActive: (teamID: string) => void;
  refresh: () => Promise<void>;
  acceptInvite: (token: string) => Promise<Membership | null>;
  /**
   * can checks whether the current viewer (in the active team) is allowed
   * to perform an action. Returns false when no active team is selected.
   * For team-scoped actions on a *different* team than active, pass the
   * membership explicitly via canIn().
   */
  can: (action: TeamAction) => boolean;
  canIn: (membership: Membership | null | undefined, action: TeamAction) => boolean;
};

const ACTIVE_TEAM_KEY = `${process.env.EXPO_PUBLIC_APP_SLUG ?? "app"}_active_team`;

const TeamsContext = createContext<TeamsContextValue | null>(null);

async function readStoredActive(): Promise<string | null> {
  if (Platform.OS === "web") {
    return globalThis.localStorage?.getItem(ACTIVE_TEAM_KEY) ?? null;
  }
  return SecureStore.getItemAsync(ACTIVE_TEAM_KEY);
}

async function writeStoredActive(teamID: string | null) {
  if (Platform.OS === "web") {
    if (teamID) {
      globalThis.localStorage?.setItem(ACTIVE_TEAM_KEY, teamID);
    } else {
      globalThis.localStorage?.removeItem(ACTIVE_TEAM_KEY);
    }
    return;
  }
  if (teamID) {
    await SecureStore.setItemAsync(ACTIVE_TEAM_KEY, teamID);
    return;
  }
  await SecureStore.deleteItemAsync(ACTIVE_TEAM_KEY);
}

// resolveCan is the single source of truth for client-side permission gates.
// Each branch maps an action to the same role tier the server enforces, with
// extra membership-shape checks (personal team / owner-only / etc).
function resolveCan(m: Membership, action: TeamAction): boolean {
  const role: TeamRole = m.role;
  switch (action) {
    case "members.list":
    case "invites.list":
      // Every member can read the roster + pending invites.
      return true;

    case "team.update":
    case "members.changeRole":
    case "members.remove":
    case "invites.create":
    case "invites.resend":
    case "invites.revoke":
    case "agent.viewAllSessions":
      return roleAtLeast(role, "admin");

    case "team.delete":
    case "team.transferOwnership":
      // Only owner; further blocked server-side for personal teams.
      return role === "owner" && !m.team.is_personal;

    case "members.leave":
      // Owners can't leave (must transfer first); personal team can't be left.
      return role !== "owner" && !m.team.is_personal;

    default:
      return false;
  }
}

// pickPreferredActive favours (in order): the previously stored active team if
// the user is still a member, the personal team, then the first membership.
// This keeps the UI deterministic across cold starts without forcing the user
// to re-pick a team after every login.
function pickPreferredActive(
  memberships: Membership[],
  storedID: string | null,
): Membership | null {
  if (memberships.length === 0) return null;
  if (storedID) {
    const found = memberships.find((m) => m.team.id === storedID);
    if (found) return found;
  }
  const personal = memberships.find((m) => m.team.is_personal);
  return personal ?? memberships[0];
}

export function TeamsProvider({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuthSession();

  const [memberships, setMemberships] = useState<Membership[]>([]);
  const [active, setActiveState] = useState<Membership | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // activeRef stays in sync with active so the api-layer provider returns the
  // current value without re-binding on every render.
  const activeRef = useRef<Membership | null>(null);

  useEffect(() => {
    activeRef.current = active;
  }, [active]);

  useEffect(() => {
    setActiveTeamProvider(() => activeRef.current?.team.id ?? null);
    return () => setActiveTeamProvider(null);
  }, []);

  const refresh = useCallback(async () => {
    if (!isAuthenticated) {
      setMemberships([]);
      setActiveState(null);
      return;
    }
    const list = await listMyTeams();
    setMemberships(list);
    const stored = await readStoredActive();
    setActiveState((current) => {
      // Keep the current selection if it's still in the list. Otherwise fall
      // back to the stored preference / personal team / first.
      if (current && list.some((m) => m.team.id === current.team.id)) {
        return current;
      }
      const preferred = pickPreferredActive(list, stored);
      void writeStoredActive(preferred?.team.id ?? null);
      return preferred;
    });
  }, [isAuthenticated]);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    refresh()
      .catch(() => {
        // Surface no-op; downstream UI will show empty / retry options.
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [refresh]);

  const setActive = useCallback(
    (teamID: string) => {
      const target = memberships.find((m) => m.team.id === teamID);
      if (!target) return;
      setActiveState(target);
      void writeStoredActive(teamID);
    },
    [memberships],
  );

  // acceptInvite turns a one-time invite token into a new membership and
  // switches the active team to the freshly-joined team so the user lands in
  // the right context immediately.
  const acceptInvite = useCallback(async (token: string) => {
    const result = await acceptInviteAPI(token);
    const list = await listMyTeams();
    setMemberships(list);
    const joined = list.find((m) => m.team.id === result.team.id) ?? null;
    if (joined) {
      setActiveState(joined);
      await writeStoredActive(joined.team.id);
    }
    return joined;
  }, []);

  const canIn = useCallback(
    (membership: Membership | null | undefined, action: TeamAction): boolean => {
      if (!membership) return false;
      return resolveCan(membership, action);
    },
    [],
  );

  const can = useCallback(
    (action: TeamAction): boolean => canIn(active, action),
    [canIn, active],
  );

  const value = useMemo<TeamsContextValue>(
    () => ({
      isLoading,
      memberships,
      active,
      setActive,
      refresh,
      acceptInvite,
      can,
      canIn,
    }),
    [isLoading, memberships, active, setActive, refresh, acceptInvite, can, canIn],
  );

  return <TeamsContext.Provider value={value}>{children}</TeamsContext.Provider>;
}

export function useTeams() {
  const ctx = useContext(TeamsContext);
  if (!ctx) {
    throw new Error("useTeams must be used within TeamsProvider");
  }
  return ctx;
}
