import { useCallback, useState } from "react";
import { ActivityIndicator, Alert, Pressable, RefreshControl, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";
import { ArrowLeft, UserMinus } from "lucide-react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Dialog } from "@/components/ui/Dialog";
import { RoleBadge } from "@/components/ui/RoleBadge";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import { TeamSegmentBar } from "@/components/teams/TeamSegmentBar";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useTeamPage } from "@/providers/TeamPageProvider";
import { removeMember, type TeamMember, updateMemberRole } from "@/services/teams";

// Members tab — roster + role changes + remove. Per-row gating funnels through
// can("members.changeRole") / can("members.remove") so the rule lives in one
// place (TeamsProvider.resolveCan).
export default function TeamMembersScreen() {
  const router = useRouter();
  const { user } = useAuthSession();
  const { membership, members, loading, refresh, can } = useTeamPage();

  const [refreshing, setRefreshing] = useState(false);
  const [busyMember, setBusyMember] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<TeamMember | null>(null);
  const [actionBusy, setActionBusy] = useState(false);

  const handleChangeRole = useCallback(
    async (member: TeamMember, next: "admin" | "member") => {
      if (!membership) return;
      setBusyMember(member.user_id);
      try {
        await updateMemberRole(membership.team.id, member.user_id, next);
        await refresh();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to update role";
        Alert.alert("Update failed", message);
      } finally {
        setBusyMember(null);
      }
    },
    [membership, refresh],
  );

  const handleRemove = useCallback(
    async (member: TeamMember) => {
      if (!membership) return;
      setBusyMember(member.user_id);
      try {
        await removeMember(membership.team.id, member.user_id);
        await refresh();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to remove";
        Alert.alert("Remove failed", message);
      } finally {
        setBusyMember(null);
      }
    },
    [membership, refresh],
  );

  const confirmRemove = useCallback(async () => {
    if (!removeTarget) return;
    setActionBusy(true);
    try {
      await handleRemove(removeTarget);
      setRemoveTarget(null);
    } finally {
      setActionBusy(false);
    }
  }, [removeTarget, handleRemove]);

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

  const showInvites = can("invites.create");
  const isOwner = membership.role === "owner";
  const canChangeRole = can("members.changeRole");
  const canRemoveMember = can("members.remove");

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
      <Pressable
        className="mb-3 flex-row items-center gap-2"
        onPress={() => router.replace(`/(app)/teams/${membership.team.id}`)}
      >
        <ArrowLeft size={16} color="#9AA4B2" />
        <Text variant="muted">Back to overview</Text>
      </Pressable>

      <View className="mb-4">
        <Text variant="h2" numberOfLines={1}>
          {membership.team.name}
        </Text>
      </View>

      <View className="mb-4">
        <TeamSegmentBar teamID={membership.team.id} showInvites={showInvites} />
      </View>

      <Card>
        <CardHeader>
          <CardTitle>Members ({members.length})</CardTitle>
        </CardHeader>
        <CardContent>
          {members.map((m, idx) => {
            const isMe = m.user_id === user?.id;
            const ownerActingOnSelf = isOwner && isMe;
            const canActOn =
              canRemoveMember &&
              !ownerActingOnSelf &&
              (isOwner ? m.role !== "owner" : m.role === "member");
            const canPromote = canChangeRole && canActOn && isOwner;
            return (
              <View key={m.user_id}>
                {idx > 0 ? <Separator /> : null}
                <View className="flex-row items-center justify-between py-2">
                  <View className="flex-1 pr-3">
                    <Text variant="p" numberOfLines={1}>
                      {m.name || m.email} {isMe ? "(you)" : ""}
                    </Text>
                    <Text variant="muted" numberOfLines={1}>
                      {m.email}
                    </Text>
                  </View>
                  <View className="flex-row items-center gap-2">
                    <RoleBadge role={m.role} />
                    {canActOn ? (
                      <>
                        {m.role === "member" && canPromote ? (
                          <Button
                            size="sm"
                            variant="outline"
                            loading={busyMember === m.user_id}
                            onPress={() => handleChangeRole(m, "admin")}
                          >
                            Make admin
                          </Button>
                        ) : null}
                        {m.role === "admin" && canPromote ? (
                          <Button
                            size="sm"
                            variant="outline"
                            loading={busyMember === m.user_id}
                            onPress={() => handleChangeRole(m, "member")}
                          >
                            Demote
                          </Button>
                        ) : null}
                        <Button
                          size="sm"
                          variant="destructive"
                          loading={busyMember === m.user_id}
                          onPress={() => setRemoveTarget(m)}
                          icon={<UserMinus size={14} color="#06070A" />}
                        >
                          Remove
                        </Button>
                      </>
                    ) : null}
                  </View>
                </View>
              </View>
            );
          })}
        </CardContent>
      </Card>

      <Dialog
        open={Boolean(removeTarget)}
        onOpenChange={(o) => (o ? null : setRemoveTarget(null))}
        title="Remove member"
        description={
          removeTarget ? `Remove ${removeTarget.email} from ${membership.team.name}?` : ""
        }
        confirmLabel="Remove"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={confirmRemove}
      />
    </ScrollView>
  );
}
