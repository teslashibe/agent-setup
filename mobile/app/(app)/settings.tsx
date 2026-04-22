import { Alert, Linking, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";

import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { RoleBadge } from "@/components/ui/RoleBadge";
import { Select, type SelectOption } from "@/components/ui/Select";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useNotificationCapture } from "@/providers/NotificationCaptureProvider";
import { useTeams } from "@/providers/TeamsProvider";
import { NOTIFICATIONS_CAPTURE_ENABLED, TEAMS_ENABLED } from "@/config";

export default function SettingsScreen() {
  const router = useRouter();
  const { user, logout } = useAuthSession();
  const { active, memberships, setActive } = useTeams();
  const capture = useNotificationCapture();

  // Team-switcher options live next to the row so we can show role + a "·"
  // separator the same way the inline picker on the home screen would.
  // L2: this is the native settings tappable team-switcher row.
  const switcherOptions: SelectOption<string>[] = memberships.map((m) => ({
    value: m.team.id,
    label: m.team.name,
    description: `${m.team.is_personal ? "Personal" : "Team"} · ${m.role}`,
  }));

  const handleLogout = async () => {
    try {
      await logout();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Could not log out";
      Alert.alert("Logout failed", message);
    }
  };

  return (
    <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 20, paddingBottom: 120 }}>
      <View className="gap-4">
        <Card>
          <CardHeader>
            <CardTitle>Profile</CardTitle>
          </CardHeader>
          <CardContent className="gap-4">
            <View className="flex-row items-center gap-3">
              <Avatar fallback={user?.name ?? "U"} />
              <View className="gap-1">
                <Text variant="large">{user?.name ?? "Unknown user"}</Text>
                <Text variant="small" className="text-muted">
                  {user?.email ?? "No email"}
                </Text>
              </View>
            </View>
            <Separator />
            <Text variant="small" className="text-muted">
              Claude Agent Go boilerplate. Sessions and messages are persisted to TimescaleDB on your backend.
            </Text>
          </CardContent>
        </Card>

        {TEAMS_ENABLED ? (
        <Card>
          <CardHeader>
            <CardTitle>Teams</CardTitle>
          </CardHeader>
          <CardContent className="gap-2">
            {active ? (
              <Select<string>
                value={active.team.id}
                onValueChange={(id) => setActive(id)}
                options={switcherOptions}
                renderTrigger={() => (
                  <View className="flex-1 flex-row items-center justify-between gap-3">
                    <View className="flex-1 pr-2">
                      <Text variant="p" numberOfLines={1}>
                        {active.team.name}
                      </Text>
                      <Text variant="small" className="text-muted" numberOfLines={1}>
                        Active team · {memberships.length} membership
                        {memberships.length === 1 ? "" : "s"}
                      </Text>
                    </View>
                    <RoleBadge role={active.role} />
                  </View>
                )}
              />
            ) : (
              <Text variant="small" className="text-muted">
                No active team yet.
              </Text>
            )}
            <Button variant="outline" size="sm" onPress={() => router.push("/(app)/teams")}>
              Manage teams
            </Button>
          </CardContent>
        </Card>
        ) : null}

        <Card>
          <CardHeader>
            <CardTitle>Platform Connections</CardTitle>
          </CardHeader>
          <CardContent className="gap-3">
            <Text variant="small" className="text-muted">
              Connect LinkedIn, X, Reddit, Instagram and 9 other platforms so the agent can act on your behalf.
              Credentials are encrypted at rest.
            </Text>
            <Button onPress={() => router.push("/(app)/platforms")} size="sm">
              Manage platform connections
            </Button>
          </CardContent>
        </Card>

        {NOTIFICATIONS_CAPTURE_ENABLED && capture.isAvailable ? (
          <Card>
            <CardHeader>
              <CardTitle>Notification Capture</CardTitle>
            </CardHeader>
            <CardContent className="gap-3">
              <Text variant="small" className="text-muted">
                {capture.isEnabled
                  ? `Capturing notifications from ${capture.allowlist.length} app${capture.allowlist.length === 1 ? "" : "s"}. The agent uses these to produce your daily rollup.`
                  : "Disabled. Enable to let the agent summarise your texts, WhatsApp, email and more."}
              </Text>
              <Button onPress={() => router.push("/(app)/capture")} size="sm">
                Manage capture settings
              </Button>
            </CardContent>
          </Card>
        ) : null}

        <Card>
          <CardHeader>
            <CardTitle>About</CardTitle>
          </CardHeader>
          <CardContent className="gap-2">
            <Text variant="small" className="text-muted">
              Built with Expo + Fiber v2 + anthropic-sdk-go.
            </Text>
            <Button
              variant="ghost"
              size="sm"
              onPress={() => Linking.openURL("https://github.com/teslashibe/agent-setup")}
            >
              View source on GitHub
            </Button>
          </CardContent>
        </Card>

        <Button variant="destructive" onPress={handleLogout}>
          Sign out
        </Button>
      </View>
    </ScrollView>
  );
}
