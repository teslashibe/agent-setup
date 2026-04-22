import { useCallback, useState } from "react";
import { ActivityIndicator, Alert, FlatList, Pressable, RefreshControl, View } from "react-native";
import { useFocusEffect, useRouter } from "expo-router";
import { Plus, Users, Check } from "lucide-react-native";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Text } from "@/components/ui/Text";
import { useTeams } from "@/providers/TeamsProvider";
import { createTeam, type Membership } from "@/services/teams";

// roleLabel maps role enum → friendly label rendered in badges and rows.
const roleLabel: Record<Membership["role"], string> = {
  owner: "Owner",
  admin: "Admin",
  member: "Member",
};

const roleVariant: Record<Membership["role"], "default" | "secondary" | "outline"> = {
  owner: "default",
  admin: "secondary",
  member: "outline",
};

export default function TeamsIndexScreen() {
  const router = useRouter();
  const { memberships, active, setActive, refresh, isLoading } = useTeams();

  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [refreshing, setRefreshing] = useState(false);

  // Refresh on focus so newly-accepted invites or remote changes show up
  // without forcing the user to pull-to-refresh manually.
  useFocusEffect(
    useCallback(() => {
      void refresh().catch(() => undefined);
    }, [refresh]),
  );

  const handleCreate = useCallback(async () => {
    const name = newName.trim();
    if (!name) {
      Alert.alert("Name required", "Give your team a name.");
      return;
    }
    setCreating(true);
    try {
      const created = await createTeam(name);
      setNewName("");
      setShowCreate(false);
      await refresh();
      // Switch into the freshly created team — that's almost always what the
      // user wants right after hitting Create.
      setActive(created.team.id);
      router.push(`/(app)/teams/${created.team.id}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create team";
      Alert.alert("Create failed", message);
    } finally {
      setCreating(false);
    }
  }, [newName, refresh, router, setActive]);

  if (isLoading && memberships.length === 0) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  return (
    <View className="flex-1 bg-background">
      <View className="flex-row items-center justify-between px-5 pt-12 pb-4">
        <View>
          <Text variant="h2">Teams</Text>
          <Text variant="muted">Switch context or invite collaborators</Text>
        </View>
        <Button
          size="sm"
          onPress={() => setShowCreate((v) => !v)}
          icon={<Plus size={16} color="#06070A" />}
        >
          {showCreate ? "Close" : "New"}
        </Button>
      </View>

      {showCreate ? (
        <View className="px-5 pb-4">
          <Card>
            <CardContent>
              <Input
                label="Team name"
                value={newName}
                onChangeText={setNewName}
                placeholder="e.g. Acme Engineering"
                autoFocus
              />
              <Button loading={creating} onPress={handleCreate}>
                Create team
              </Button>
            </CardContent>
          </Card>
        </View>
      ) : null}

      <FlatList
        data={memberships}
        keyExtractor={(item) => item.team.id}
        contentContainerStyle={{ padding: 20, paddingTop: 0, paddingBottom: 140, gap: 12 }}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            tintColor="#00D4AA"
            onRefresh={async () => {
              setRefreshing(true);
              try {
                await refresh();
              } finally {
                setRefreshing(false);
              }
            }}
          />
        }
        ListEmptyComponent={
          <EmptyState
            icon={<Users color="#00D4AA" size={28} />}
            title="No teams yet"
            description="Create a team to collaborate with others."
            actionLabel="Create team"
            onAction={() => setShowCreate(true)}
          />
        }
        renderItem={({ item }) => {
          const isActive = active?.team.id === item.team.id;
          return (
            <Pressable
              onPress={() => {
                setActive(item.team.id);
                router.push(`/(app)/teams/${item.team.id}`);
              }}
            >
              <Card className={isActive ? "border-primary" : undefined}>
                <CardContent>
                  <View className="flex-row items-center justify-between">
                    <View className="flex-1 pr-3">
                      <View className="flex-row items-center gap-2">
                        <Text variant="large" numberOfLines={1}>
                          {item.team.name}
                        </Text>
                        {item.team.is_personal ? (
                          <Badge variant="outline">Personal</Badge>
                        ) : null}
                      </View>
                      <Text variant="muted" numberOfLines={1}>
                        {item.team.slug}
                      </Text>
                    </View>
                    <View className="flex-row items-center gap-2">
                      <Badge variant={roleVariant[item.role]}>{roleLabel[item.role]}</Badge>
                      {isActive ? <Check size={16} color="#00D4AA" /> : null}
                    </View>
                  </View>
                </CardContent>
              </Card>
            </Pressable>
          );
        }}
      />
    </View>
  );
}
