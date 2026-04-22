import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";

import {
  acceptInvite as acceptInviteAPI,
  type Membership,
  listMyTeams,
} from "@/services/teams";
import { setActiveTeamProvider } from "@/services/api";
import { useAuthSession } from "@/providers/AuthSessionProvider";

type TeamsContextValue = {
  isLoading: boolean;
  memberships: Membership[];
  active: Membership | null;
  setActive: (teamID: string) => void;
  refresh: () => Promise<void>;
  acceptInvite: (token: string) => Promise<Membership | null>;
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

  const value = useMemo<TeamsContextValue>(
    () => ({ isLoading, memberships, active, setActive, refresh, acceptInvite }),
    [isLoading, memberships, active, setActive, refresh, acceptInvite],
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
