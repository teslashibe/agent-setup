import { Alert, Linking, Pressable, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";

import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useTeams } from "@/providers/TeamsProvider";

export default function SettingsScreen() {
  const router = useRouter();
  const { user, logout } = useAuthSession();
  const { active, memberships } = useTeams();

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

        <Card>
          <CardHeader>
            <CardTitle>Teams</CardTitle>
          </CardHeader>
          <CardContent className="gap-2">
            {active ? (
              <View className="flex-row items-center justify-between">
                <View className="flex-1 pr-3">
                  <Text variant="p" numberOfLines={1}>
                    {active.team.name}
                  </Text>
                  <Text variant="small" className="text-muted">
                    Active team · {memberships.length} membership{memberships.length === 1 ? "" : "s"}
                  </Text>
                </View>
                <Badge>{active.role}</Badge>
              </View>
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
