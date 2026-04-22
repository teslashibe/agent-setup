import { useCallback, useEffect, useState } from "react";
import { ActivityIndicator, Alert, View } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { CheckCircle2, MailWarning, Users } from "lucide-react-native";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useTeams } from "@/providers/TeamsProvider";
import { previewInvite, type TeamRole } from "@/services/teams";

type Preview = {
  team: { id: string; name: string };
  email: string;
  role: TeamRole;
  expires_at: string;
};

// InviteLandingScreen is the deep-link target for invitation emails. It runs
// at the URL scheme /invites/accept?token=... whether opened on web (universal
// link) or native (deep link via expo-linking). Three branches:
//
//   1. Signed-out → forward to /(auth)/welcome with email + invite_token in
//      query params; the magic-link flow auto-accepts on verify.
//   2. Signed-in with the invited email → call TeamsProvider.acceptInvite,
//      switch active team, land in /(app)/teams/:id.
//   3. Signed-in with a different email → explain and offer to log out.
export default function InviteLandingScreen() {
  const router = useRouter();
  const params = useLocalSearchParams<{ token?: string }>();
  const token = typeof params.token === "string" ? params.token : "";

  const { isAuthenticated, isLoading: authLoading, user, logout } = useAuthSession();
  const { acceptInvite } = useTeams();

  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!token) {
      setError("This invite link is missing a token.");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const p = (await previewInvite(token)) as Preview;
        if (!cancelled) setPreview(p);
      } catch (err) {
        if (!cancelled) {
          const message = err instanceof Error ? err.message : "Invite is invalid or expired.";
          setError(message);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  const goSignIn = useCallback(() => {
    if (!preview) return;
    router.replace({
      pathname: "/(auth)/welcome",
      params: { email: preview.email, invite_token: token },
    });
  }, [preview, router, token]);

  const handleAccept = useCallback(async () => {
    if (!preview) return;
    setBusy(true);
    try {
      const joined = await acceptInvite(token);
      if (joined) {
        router.replace(`/(app)/teams/${joined.team.id}`);
      } else {
        // We accepted but the membership wasn't visible yet — drop the user
        // somewhere sensible.
        router.replace("/(app)/teams");
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to accept invite";
      Alert.alert("Could not join", message);
    } finally {
      setBusy(false);
    }
  }, [acceptInvite, preview, router, token]);

  if (authLoading || (!preview && !error)) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  if (error) {
    return (
      <View className="flex-1 items-center justify-center bg-background px-5">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Invite unavailable</CardTitle>
            <CardDescription>{error}</CardDescription>
          </CardHeader>
          <CardContent>
            <Button variant="ghost" onPress={() => router.replace("/")}>
              Go home
            </Button>
          </CardContent>
        </Card>
      </View>
    );
  }

  if (!preview) return null;

  const expires = new Date(preview.expires_at);
  const wrongEmail = isAuthenticated && user?.email?.toLowerCase() !== preview.email.toLowerCase();

  return (
    <View className="flex-1 items-center justify-center bg-background px-5">
      <Card className="w-full max-w-md">
        <CardHeader>
          <View className="flex-row items-center gap-2">
            <Users color="#00D4AA" size={20} />
            <CardTitle>Join {preview.team.name}</CardTitle>
          </View>
          <CardDescription>
            You've been invited as <Text variant="small" className="font-semibold">{preview.role}</Text> to{" "}
            <Text variant="small" className="font-semibold">{preview.team.name}</Text>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <View className="flex-row items-center justify-between">
            <Text variant="small" className="text-muted">Email</Text>
            <Badge variant="outline">{preview.email}</Badge>
          </View>
          <View className="flex-row items-center justify-between">
            <Text variant="small" className="text-muted">Expires</Text>
            <Text variant="small">{expires.toLocaleDateString()}</Text>
          </View>

          {wrongEmail ? (
            <View className="mt-2 gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3">
              <View className="flex-row items-center gap-2">
                <MailWarning size={16} color="#ef4444" />
                <Text variant="small" className="font-semibold">Wrong account</Text>
              </View>
              <Text variant="small" className="text-muted">
                You're signed in as {user?.email}. This invite was sent to {preview.email}. Sign out
                and back in with that email to accept.
              </Text>
              <Button
                variant="destructive"
                size="sm"
                onPress={async () => {
                  await logout();
                  goSignIn();
                }}
              >
                Sign out and continue
              </Button>
            </View>
          ) : isAuthenticated ? (
            <Button
              loading={busy}
              icon={<CheckCircle2 size={16} color="#06070A" />}
              onPress={handleAccept}
            >
              Join team
            </Button>
          ) : (
            <Button onPress={goSignIn}>Sign in to accept</Button>
          )}
        </CardContent>
      </Card>
    </View>
  );
}
