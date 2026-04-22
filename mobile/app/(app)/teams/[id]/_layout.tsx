import { useCallback } from "react";
import { Stack, useFocusEffect, useLocalSearchParams } from "expo-router";

import { TeamPageProvider } from "@/providers/TeamPageProvider";
import { useTeams } from "@/providers/TeamsProvider";

// Wraps the per-team routes in a single TeamPageProvider so navigating
// between Overview / Members / Invites doesn't refetch the membership
// + roster + pending invites three times.
export default function TeamDetailLayout() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const teamID = Array.isArray(id) ? id[0] : id;
  const { setActive } = useTeams();

  // Whenever any sub-route under /teams/:id is focused, switch the active
  // team to match the URL — keeps the global active-team header in sync
  // with what the user is actually looking at.
  useFocusEffect(
    useCallback(() => {
      if (teamID) setActive(teamID);
    }, [teamID, setActive]),
  );

  return (
    <TeamPageProvider teamID={teamID ?? ""}>
      <Stack screenOptions={{ headerShown: false }} />
    </TeamPageProvider>
  );
}
