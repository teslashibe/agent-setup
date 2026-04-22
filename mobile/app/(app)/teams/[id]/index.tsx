import { useCallback, useState } from "react";
import { ActivityIndicator, Alert, Pressable, RefreshControl, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";
import { ArrowLeft } from "lucide-react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Dialog } from "@/components/ui/Dialog";
import { RoleBadge } from "@/components/ui/RoleBadge";
import { Text } from "@/components/ui/Text";
import { TeamSegmentBar } from "@/components/teams/TeamSegmentBar";
import { useTeamPage } from "@/providers/TeamPageProvider";
import { useTeams } from "@/providers/TeamsProvider";
import { deleteTeam, leaveTeam } from "@/services/teams";

// Overview tab — team header + danger zone (leave / delete). Members + Invites
// live in sibling routes so each surface owns its own state and refresh.
export default function TeamOverviewScreen() {
  const router = useRouter();
  const { membership, loading, refresh, can } = useTeamPage();
  const { refresh: refreshMemberships } = useTeams();

  const [refreshing, setRefreshing] = useState(false);
  const [showLeave, setShowLeave] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);

  const handleLeave = useCallback(async () => {
    if (!membership) return;
    setActionBusy(true);
    try {
      await leaveTeam(membership.team.id);
      await refreshMemberships();
      setShowLeave(false);
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to leave";
      Alert.alert("Leave failed", message);
    } finally {
      setActionBusy(false);
    }
  }, [membership, refreshMemberships, router]);

  const handleDelete = useCallback(async () => {
    if (!membership) return;
    setActionBusy(true);
    try {
      await deleteTeam(membership.team.id);
      await refreshMemberships();
      setShowDelete(false);
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete";
      Alert.alert("Delete failed", message);
    } finally {
      setActionBusy(false);
    }
  }, [membership, refreshMemberships, router]);

  if (loading && !membership) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  if (!membership) {
    return (
      <View className="flex-1 items-center justify-center bg-background px-5">
        <Text variant="muted">Team not found.</Text>
        <Button variant="ghost" onPress={() => router.back()}>
          Back
        </Button>
      </View>
    );
  }

  const canLeave = can("members.leave");
  const canDelete = can("team.delete");
  const showInvites = can("invites.create");

  return (
    <ScrollView
      className="flex-1 bg-background"
      contentContainerStyle={{ padding: 20, paddingTop: 48, paddingBottom: 140 }}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          tintColor="#00D4AA"
          onRefresh={async () => {
            setRefreshing(true);
            await refresh().catch(() => undefined);
            setRefreshing(false);
          }}
        />
      }
    >
      <Pressable className="mb-3 flex-row items-center gap-2" onPress={() => router.back()}>
        <ArrowLeft size={16} color="#9AA4B2" />
        <Text variant="muted">Back</Text>
      </Pressable>

      <View className="mb-4">
        <View className="flex-row items-center gap-2">
          <Text variant="h2" numberOfLines={1} className="flex-1">
            {membership.team.name}
          </Text>
          <RoleBadge role={membership.role} />
        </View>
        <Text variant="muted">{membership.team.slug}</Text>
      </View>

      <View className="mb-4">
        <TeamSegmentBar teamID={membership.team.id} showInvites={showInvites} />
      </View>

      <Card>
        <CardHeader>
          <CardTitle>Danger zone</CardTitle>
        </CardHeader>
        <CardContent>
          {canLeave ? (
            <Button variant="destructive" onPress={() => setShowLeave(true)}>
              Leave team
            </Button>
          ) : membership.team.is_personal ? (
            <Text variant="muted">This is your personal team and cannot be left or deleted.</Text>
          ) : null}
          {canDelete ? (
            <Button variant="destructive" onPress={() => setShowDelete(true)}>
              Delete team
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <Dialog
        open={showLeave}
        onOpenChange={setShowLeave}
        title="Leave team"
        description={`Leave ${membership.team.name}?`}
        confirmLabel="Leave"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={handleLeave}
      />

      <Dialog
        open={showDelete}
        onOpenChange={setShowDelete}
        title="Delete team"
        description={`Delete ${membership.team.name} and all its data? This cannot be undone.`}
        confirmLabel="Delete"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={handleDelete}
      />
    </ScrollView>
  );
}
