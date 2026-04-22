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

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
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
  roleAtLeast,
  type TeamMember,
  type TeamRole,
  updateMemberRole,
} from "@/services/teams";

const roleOrder: Record<TeamRole, number> = { owner: 0, admin: 1, member: 2 };
const roleLabel: Record<TeamRole, string> = { owner: "Owner", admin: "Admin", member: "Member" };
const roleVariant: Record<TeamRole, "default" | "secondary" | "outline"> = {
  owner: "default",
  admin: "secondary",
  member: "outline",
};

export default function TeamDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const teamID = Array.isArray(id) ? id[0] : id;

  const { user } = useAuthSession();
  const { setActive, refresh: refreshMemberships } = useTeams();

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

  const role = membership?.role;
  const canManage = roleAtLeast(role, "admin");
  const isOwner = role === "owner";

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
      // Only admins+ can list pending invites server-side.
      if (roleAtLeast(m.role, "admin")) {
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
  }, [teamID]);

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
    try {
      await leaveTeam(teamID);
      await refreshMemberships();
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to leave";
      Alert.alert("Leave failed", message);
    }
  }, [teamID, membership, refreshMemberships, router]);

  const handleDelete = useCallback(async () => {
    if (!teamID || !membership) return;
    if (membership.team.is_personal) {
      Alert.alert("Cannot delete", "Personal teams can't be deleted.");
      return;
    }
    try {
      await deleteTeam(teamID);
      await refreshMemberships();
      router.replace("/(app)/teams");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete";
      Alert.alert("Delete failed", message);
    }
  }, [teamID, membership, refreshMemberships, router]);

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
          <Badge variant={roleVariant[membership.role]}>{roleLabel[membership.role]}</Badge>
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
            // Owners can demote/remove anyone but themselves; admins can manage
            // members only (not other admins or the owner).
            const canActOn =
              canManage &&
              !ownerActingOnSelf &&
              (isOwner ? m.role !== "owner" : m.role === "member");
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
                    <Badge variant={roleVariant[m.role]}>{roleLabel[m.role]}</Badge>
                    {canActOn ? (
                      <>
                        {m.role === "member" && isOwner ? (
                          <Button
                            size="sm"
                            variant="outline"
                            loading={busyMember === m.user_id}
                            onPress={() => handleChangeRole(m, "admin")}
                          >
                            Make admin
                          </Button>
                        ) : null}
                        {m.role === "admin" && isOwner ? (
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
                          onPress={() =>
                            Alert.alert(
                              "Remove member",
                              `Remove ${m.email} from ${membership.team.name}?`,
                              [
                                { text: "Cancel", style: "cancel" },
                                { text: "Remove", style: "destructive", onPress: () => handleRemove(m) },
                              ],
                            )
                          }
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
      {canManage ? (
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
            <View className="flex-row items-center gap-2">
              <Text variant="small" className="text-muted">
                Role:
              </Text>
              <Pressable onPress={() => setInviteRole("member")}>
                <Badge variant={inviteRole === "member" ? "default" : "outline"}>Member</Badge>
              </Pressable>
              {isOwner ? (
                <Pressable onPress={() => setInviteRole("admin")}>
                  <Badge variant={inviteRole === "admin" ? "default" : "outline"}>Admin</Badge>
                </Pressable>
              ) : null}
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

      {canManage && invites.length > 0 ? (
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
                    <Badge variant={roleVariant[inv.role]}>{roleLabel[inv.role]}</Badge>
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
                      onPress={() =>
                        Alert.alert(
                          "Revoke invite",
                          `Revoke pending invite for ${inv.email}?`,
                          [
                            { text: "Cancel", style: "cancel" },
                            { text: "Revoke", style: "destructive", onPress: () => handleRevoke(inv) },
                          ],
                        )
                      }
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
          {!membership.team.is_personal ? (
            <Button
              variant="destructive"
              onPress={() =>
                Alert.alert(
                  "Leave team",
                  `Leave ${membership.team.name}?`,
                  [
                    { text: "Cancel", style: "cancel" },
                    { text: "Leave", style: "destructive", onPress: handleLeave },
                  ],
                )
              }
            >
              Leave team
            </Button>
          ) : (
            <Text variant="muted">This is your personal team and cannot be left or deleted.</Text>
          )}
          {isOwner && !membership.team.is_personal ? (
            <Button
              variant="destructive"
              onPress={() =>
                Alert.alert(
                  "Delete team",
                  `Delete ${membership.team.name} and all its data? This cannot be undone.`,
                  [
                    { text: "Cancel", style: "cancel" },
                    { text: "Delete", style: "destructive", onPress: handleDelete },
                  ],
                )
              }
            >
              Delete team
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </ScrollView>
  );
}
