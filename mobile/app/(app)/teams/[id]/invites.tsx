import { useCallback, useState } from "react";
import { ActivityIndicator, Alert, Pressable, RefreshControl, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";
import { ArrowLeft, Mail, RotateCw, Trash2, UserPlus } from "lucide-react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Dialog } from "@/components/ui/Dialog";
import { Input } from "@/components/ui/Input";
import { RoleBadge } from "@/components/ui/RoleBadge";
import { Select, type SelectOption } from "@/components/ui/Select";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import { TeamSegmentBar } from "@/components/teams/TeamSegmentBar";
import { useTeamPage } from "@/providers/TeamPageProvider";
import {
  createInvite,
  type Invite,
  resendInvite,
  revokeInvite,
  type TeamRole,
} from "@/services/teams";

const inviteRoleOptions: SelectOption<Exclude<TeamRole, "owner">>[] = [
  { value: "member", label: "Member", description: "Read + use the agent" },
  { value: "admin", label: "Admin", description: "Manage members + invites" },
];

// Invites tab — admin-only. Server returns 403 to non-admins on the invite
// list, but we also gate the route here for consistency: the segment bar
// hides the tab when can("invites.create") is false, so this screen is
// only reachable by admins+ in normal flow.
export default function TeamInvitesScreen() {
  const router = useRouter();
  const { membership, invites, loading, refresh, can } = useTeamPage();

  const [refreshing, setRefreshing] = useState(false);
  const [busyInvite, setBusyInvite] = useState<string | null>(null);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<Exclude<TeamRole, "owner">>("member");
  const [creatingInvite, setCreatingInvite] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<Invite | null>(null);
  const [actionBusy, setActionBusy] = useState(false);

  const handleInvite = useCallback(async () => {
    const email = inviteEmail.trim();
    if (!email) {
      Alert.alert("Email required", "Enter an email address.");
      return;
    }
    if (!membership) return;
    setCreatingInvite(true);
    try {
      await createInvite(membership.team.id, email, inviteRole);
      setInviteEmail("");
      await refresh();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to invite";
      Alert.alert("Invite failed", message);
    } finally {
      setCreatingInvite(false);
    }
  }, [inviteEmail, inviteRole, membership, refresh]);

  const handleResend = useCallback(
    async (inv: Invite) => {
      if (!membership) return;
      setBusyInvite(inv.id);
      try {
        await resendInvite(membership.team.id, inv.id);
        Alert.alert("Resent", `Email sent to ${inv.email}.`);
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to resend";
        Alert.alert("Resend failed", message);
      } finally {
        setBusyInvite(null);
      }
    },
    [membership],
  );

  const handleRevoke = useCallback(
    async (inv: Invite) => {
      if (!membership) return;
      setBusyInvite(inv.id);
      try {
        await revokeInvite(membership.team.id, inv.id);
        await refresh();
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to revoke";
        Alert.alert("Revoke failed", message);
      } finally {
        setBusyInvite(null);
      }
    },
    [membership, refresh],
  );

  const confirmRevoke = useCallback(async () => {
    if (!revokeTarget) return;
    setActionBusy(true);
    try {
      await handleRevoke(revokeTarget);
      setRevokeTarget(null);
    } finally {
      setActionBusy(false);
    }
  }, [revokeTarget, handleRevoke]);

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
  if (!can("invites.create")) {
    return (
      <View className="flex-1 items-center justify-center bg-background px-5">
        <Text variant="muted" className="mb-2">
          Only admins can manage invites.
        </Text>
        <Button variant="ghost" onPress={() => router.replace(`/(app)/teams/${membership.team.id}`)}>
          Back to overview
        </Button>
      </View>
    );
  }

  const isOwner = membership.role === "owner";

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
        <TeamSegmentBar teamID={membership.team.id} showInvites />
      </View>

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
              // Owners can grant admin; admins can only invite members.
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

      {invites.length > 0 ? (
        <Card>
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
      ) : (
        <Text variant="muted">No pending invites.</Text>
      )}

      <Dialog
        open={Boolean(revokeTarget)}
        onOpenChange={(o) => (o ? null : setRevokeTarget(null))}
        title="Revoke invite"
        description={revokeTarget ? `Revoke pending invite for ${revokeTarget.email}?` : ""}
        confirmLabel="Revoke"
        confirmVariant="destructive"
        confirmLoading={actionBusy}
        onConfirm={confirmRevoke}
      />
    </ScrollView>
  );
}
