import { useCallback, useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  RefreshControl,
  ScrollView,
  View,
} from "react-native";
import { useFocusEffect, useLocalSearchParams, useRouter } from "expo-router";
import { ArrowLeft, Mail, RotateCw, Trash2, UserMinus, UserPlus } from "lucide-react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Dialog } from "@/components/ui/Dialog";
import { Input } from "@/components/ui/Input";
import { RoleBadge } from "@/components/ui/RoleBadge";
import { Select, type SelectOption } from "@/components/ui/Select";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useTeams } from "@/providers/TeamsProvider";
import {
  createInvite,
  deleteTeam,
  getTeam,
  type Invite,
  leaveTeam,
  listInvites,
  listMembers,
  type Membership,
  removeMember,
  resendInvite,
  revokeInvite,
  type TeamMember,
  type TeamRole,
  updateMemberRole,
} from "@/services/teams";

const roleOrder: Record<TeamRole, number> = { owner: 0, admin: 1, member: 2 };

const inviteRoleOptions: SelectOption<Exclude<TeamRole, "owner">>[] = [
  { value: "member", label: "Member", description: "Read + use the agent" },
  { value: "admin", label: "Admin", description: "Manage members + invites" },
];

export default function TeamDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const teamID = Array.isArray(id) ? id[0] : id;

  const { user } = useAuthSession();
  const { setActive, refresh: refreshMemberships, canIn } = useTeams();

  const [membership, setMembership] = useState<Membership | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busyMember, setBusyMember] = useState<string | null>(null);
  const [busyInvite, setBusyInvite] = useState<string | null>(null);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<Exclude<TeamRole, "owner">>("member");
  const [creatingInvite, setCreatingInvite] = useState(false);
  // Dialog state — one at a time. Storing the target object on the state
  // avoids closures over stale .map() iteration variables.
  const [removeTarget, setRemoveTarget] = useState<TeamMember | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<Invite | null>(null);
  const [showLeave, setShowLeave] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);

  // All client-side gates funnel through canIn() — see TeamsProvider.resolveCan
  // for the central rule table that mirrors the server's permission checks.
  const role = membership?.role;
  const isOwner = role === "owner";
  const canInviteCreate = canIn(membership, "invites.create");
  const canInviteList = canIn(membership, "invites.list") && canIn(membership, "invites.create");
  const canChangeRole = canIn(membership, "members.changeRole");
  const canRemoveMember = canIn(membership, "members.remove");
  const canLeave = canIn(membership, "members.leave");
  const canDelete = canIn(membership, "team.delete");

  // Whenever this screen is focused, switch the active team to match the URL.
  // That way the header / sidebar's "active team" pill stays in sync with what
  // the user is actually looking at.
  useFocusEffect(
    useCallback(() => {
      if (teamID) setActive(teamID);
    }, [teamID, setActive]),
  );

  const load = useCallback(async () => {
    if (!teamID) return;
    try {
      const [m, mems] = await Promise.all([getTeam(teamID), listMembers(teamID)]);
      setMembership(m);
      setMembers(
        [...mems].sort((a, b) => roleOrder[a.role] - roleOrder[b.role] || a.email.localeCompare(b.email)),
      );
      // Only admins+ can list pending invites server-side; mirror that gate
      // here so we don't waste a request that we know will 403.
      if (canIn(m, "invites.list")) {
        const inv = await listInvites(teamID);
        setInvites(inv);
      } else {
        setInvites([]);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load team";
      Alert.alert("Error", message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [teamID, canIn]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleInvite = useCallback(async () => {
    const email = inviteEmail.trim();
    if (!email) {
      Alert.alert("Email required", "Enter an email address.");
      return;
    }
    if (!teamID) return;
    setCreatingInvite(true);
    try {
      await createInvite(teamID, email, inviteRole);
      setInviteEmail("");
      await load();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to invite";
      Alert.alert("Invite failed", message);
    } finally {
      setCreatingInvite(false);
    }
  }, [inviteEmail, inviteRole, teamID, load]);

  const handleResend = useCallback(
    async (inv: Invite) => {
      if (!teamID) return;
      setBusyInvite(inv.id);
      try {
        await resendInvite(teamID, inv.id);
        Alert.alert("Resent", `Email sent to ${inv.email}.`);
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to resend";
        Alert.alert("Resend failed", message);
      } finally {
        setBusyInvite(null);
      }
    },
    [teamID],
  );

  const handleRevoke = useCallback(
    async (inv: Invite) => {
      if (!teamID) return;
      setBusyInvite(inv.id);
      try {
        await revokeInvite(teamID, inv.id);
        await load();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to revoke";
        Alert.alert("Revoke failed", message);
      } finally {
        setBusyInvite(null);
      }
    },
    [teamID, load],
  );

  const handleChangeRole = useCallback(
    async (member: TeamMember, next: Exclude<TeamRole, "owner">) => {
      if (!teamID) return;
      setBusyMember(member.user_id);
      try {
        await updateMemberRole(teamID, member.user_id, next);
        await load();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to update role";
        Alert.alert("Update failed", message);
      } finally {
        setBusyMember(null);
      }
    },
    [teamID, load],
  );

  const handleRemove = useCallback(
    async (member: TeamMember) => {
      if (!teamID) return;
      setBusyMember(member.user_id);
      try {
        await removeMember(teamID, member.user_id);
        await load();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to remove";
        Alert.alert("Remove failed", message);
      } finally {
        setBusyMember(null);
      }
    },
    [teamID, load],
  );

  const handleLeave = useCallback(async () => {
    if (!teamID || !membership) return;
    if (membership.team.is_personal) {
      Alert.alert("Cannot leave", "You can't leave your personal team.");
      return;
    }
    setActionBusy(true);
    try {
      await leaveTeam(teamID);
      await refreshMemberships();
      setShowLeave(false);
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to leave";
      Alert.alert("Leave failed", message);
    } finally {
      setActionBusy(false);
    }
  }, [teamID, membership, refreshMemberships, router]);

  const handleDelete = useCallback(async () => {
    if (!teamID || !membership) return;
    if (membership.team.is_personal) {
      Alert.alert("Cannot delete", "Personal teams can't be deleted.");
      return;
    }
    setActionBusy(true);
    try {
      await deleteTeam(teamID);
      await refreshMemberships();
      setShowDelete(false);
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete";
      Alert.alert("Delete failed", message);
    } finally {
      setActionBusy(false);
    }
  }, [teamID, membership, refreshMemberships, router]);

  // confirmRemoveMember + confirmRevokeInvite drive the Dialog from the row
  // buttons below. Splitting "show the prompt" from "actually do the work"
  // keeps the per-row click handlers tiny and reuses handleRemove/handleRevoke.
  const confirmRemoveMember = useCallback(async () => {
    if (!removeTarget) return;
    setActionBusy(true);
    try {
      await handleRemove(removeTarget);
      setRemoveTarget(null);
    } finally {
      setActionBusy(false);
    }
  }, [removeTarget, handleRemove]);

  const confirmRevokeInvite = useCallback(async () => {
    if (!revokeTarget) return;
    setActionBusy(true);
    try {
      await handleRevoke(revokeTarget);
      setRevokeTarget(null);
    } finally {
      setActionBusy(false);
    }
  }, [revokeTarget, handleRevoke]);

  if (loading) {
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
            await load();
          }}
        />
      }
    >
      <Pressable className="mb-3 flex-row items-center gap-2" onPress={() => router.back()}>
        <ArrowLeft size={16} color="#9AA4B2" />
        <Text variant="muted">Back</Text>
      </Pressable>

      <View className="mb-6">
        <View className="flex-row items-center gap-2">
          <Text variant="h2" numberOfLines={1} className="flex-1">
            {membership.team.name}
          </Text>
          <RoleBadge role={membership.role} />
        </View>
        <Text variant="muted">{membership.team.slug}</Text>
      </View>

      {/* Members */}
      <Card className="mb-4">
        <CardHeader>
          <CardTitle>Members ({members.length})</CardTitle>
        </CardHeader>
        <CardContent>
          {members.map((m, idx) => {
            const isMe = m.user_id === user?.id;
            const ownerActingOnSelf = isOwner && isMe;
            // Owners can manage anyone but themselves; admins can manage
            // members only (not other admins or the owner). canRemoveMember
            // gives us the role tier; the per-row checks scope the target.
            const canActOn =
              canRemoveMember &&
              !ownerActingOnSelf &&
              (isOwner ? m.role !== "owner" : m.role === "member");
            const canPromoteThisMember = canChangeRole && canActOn && isOwner;
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
                        {m.role === "member" && canPromoteThisMember ? (
                          <Button
                            size="sm"
                            variant="outline"
                            loading={busyMember === m.user_id}
                            onPress={() => handleChangeRole(m, "admin")}
                          >
                            Make admin
                          </Button>
                        ) : null}
                        {m.role === "admin" && canPromoteThisMember ? (
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

      {/* Invites */}
      {canInviteCreate ? (
        <Card className="mb-4">
          <CardHeader>
            <CardTitle>Invite a teammate</CardTitle>
          </CardHeader>
          <CardContent>
            <Input
              label="Email"
              value={inviteEmail}
              onChangeText={setInviteEmail}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="email-address"
              placeholder="teammate@example.com"
            />
            <View className="gap-2">
              <Text variant="small" className="text-muted">
                Role
              </Text>
              <Select<Exclude<TeamRole, "owner">>
                value={inviteRole}
                onValueChange={setInviteRole}
                // Only owners can grant admin (mirrors server canActOn).
                options={
                  isOwner
                    ? inviteRoleOptions
                    : inviteRoleOptions.filter((o) => o.value === "member")
                }
              />
            </View>
            <Button
              icon={<UserPlus size={14} color="#06070A" />}
              loading={creatingInvite}
              onPress={handleInvite}
            >
              Send invite
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {canInviteList && invites.length > 0 ? (
        <Card className="mb-4">
          <CardHeader>
            <CardTitle>Pending invites ({invites.length})</CardTitle>
          </CardHeader>
          <CardContent>
            {invites.map((inv, idx) => (
              <View key={inv.id}>
                {idx > 0 ? <Separator /> : null}
                <View className="flex-row items-center justify-between py-2">
                  <View className="flex-1 pr-3">
                    <View className="flex-row items-center gap-2">
                      <Mail size={14} color="#9AA4B2" />
                      <Text variant="p" numberOfLines={1}>
                        {inv.email}
                      </Text>
                    </View>
                    <Text variant="muted">
                      Expires {new Date(inv.expires_at).toLocaleDateString()}
                    </Text>
                  </View>
                  <View className="flex-row items-center gap-2">
                    <RoleBadge role={inv.role} />
                    <Button
                      size="sm"
                      variant="outline"
                      loading={busyInvite === inv.id}
                      onPress={() => handleResend(inv)}
                      icon={<RotateCw size={14} color="#9AA4B2" />}
                    >
                      Resend
                    </Button>
                    <Button
                      size="sm"
                      variant="destructive"
                      loading={busyInvite === inv.id}
                      onPress={() => setRevokeTarget(inv)}
                      icon={<Trash2 size={14} color="#06070A" />}
                    >
                      Revoke
                    </Button>
                  </View>
                </View>
              </View>
            ))}
          </CardContent>
        </Card>
      ) : null}

      {/* Danger zone */}
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

      {/* Confirmation dialogs — Dialog primitive replaces Alert.alert so the
          UI works on web + matches the app's dark theme on iOS / Android. */}
      <Dialog
        open={Boolean(removeTarget)}
        onOpenChange={(o) => (o ? null : setRemoveTarget(null))}
        title="Remove member"
        description={
          removeTarget
            ? `Remove ${removeTarget.email} from ${membership.team.name}?`
            : ""
        }
        confirmLabel="Remove"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={confirmRemoveMember}
      />

      <Dialog
        open={Boolean(revokeTarget)}
        onOpenChange={(o) => (o ? null : setRevokeTarget(null))}
        title="Revoke invite"
        description={
          revokeTarget
            ? `Revoke pending invite for ${revokeTarget.email}?`
            : ""
        }
        confirmLabel="Revoke"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={confirmRevokeInvite}
      />

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
