import { useCallback, useEffect, useState } from "react";
import { ActivityIndicator, Alert, FlatList, Pressable, RefreshControl, View } from "react-native";
import { useFocusEffect, useRouter } from "expo-router";
import { MessageSquarePlus, Sparkles } from "lucide-react-native";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { useTeams } from "@/providers/TeamsProvider";
import { createSession, listSessions, type ListScope, type Session } from "@/services/agent";

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  const seconds = Math.max(1, Math.floor((Date.now() - then) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function SessionsScreen() {
  const router = useRouter();
  const { user } = useAuthSession();
  const { active, can } = useTeams();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [creating, setCreating] = useState(false);
  const [scope, setScope] = useState<ListScope>("mine");

  // Only admins+ can list "all" sessions in the active team. Funnels through
  // the central can() table so the rule lives in one place.
  const canSeeAll = can("agent.viewAllSessions");
  const effectiveScope: ListScope = canSeeAll ? scope : "mine";

  const load = useCallback(async () => {
    try {
      const data = await listSessions(effectiveScope);
      setSessions(data);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to load sessions";
      Alert.alert("Error", message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [effectiveScope]);

  useFocusEffect(
    useCallback(() => {
      void load();
    }, [load])
  );

  // Reload when the active team or scope changes so the list always reflects
  // the current X-Team-ID context.
  useEffect(() => {
    void load();
  }, [active?.team.id, load]);

  const handleNew = useCallback(async () => {
    setCreating(true);
    try {
      const session = await createSession("New chat");
      router.push(`/(app)/chat/${session.id}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create session";
      Alert.alert("Error", message);
    } finally {
      setCreating(false);
    }
  }, [router]);

  if (loading) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  return (
    <View className="flex-1 bg-background">
      <View className="flex-row items-center justify-between px-5 pt-12 pb-2">
        <View className="flex-1 pr-3">
          <Text variant="h2">Chats</Text>
          <Text variant="muted" numberOfLines={1}>
            {active ? active.team.name : "Your agent conversations"}
          </Text>
        </View>
        <Button size="sm" loading={creating} onPress={handleNew} icon={<MessageSquarePlus size={16} color="#06070A" />}>
          New
        </Button>
      </View>

      {canSeeAll ? (
        <View className="flex-row items-center gap-2 px-5 pb-3">
          <Pressable onPress={() => setScope("mine")}>
            <Badge variant={scope === "mine" ? "default" : "outline"}>Mine</Badge>
          </Pressable>
          <Pressable onPress={() => setScope("all")}>
            <Badge variant={scope === "all" ? "default" : "outline"}>All in team</Badge>
          </Pressable>
        </View>
      ) : null}

      <FlatList
        data={sessions}
        keyExtractor={(item) => item.id}
        contentContainerStyle={{ padding: 20, paddingTop: 0, paddingBottom: 140, gap: 12 }}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            tintColor="#00D4AA"
            onRefresh={() => {
              setRefreshing(true);
              void load();
            }}
          />
        }
        ListEmptyComponent={
          <EmptyState
            icon={<Sparkles color="#00D4AA" size={28} />}
            title="No chats yet"
            description="Start a new conversation to talk with your Claude agent."
            actionLabel="Start a chat"
            onAction={handleNew}
          />
        }
        renderItem={({ item }) => {
          // In "all" scope, show whose session this is so admins can tell
          // their own from other members'.
          const otherUser = scope === "all" && user && item.user_id !== user.id;
          return (
            <Pressable onPress={() => router.push(`/(app)/chat/${item.id}`)}>
              <Card>
                <CardContent>
                  <View className="flex-row items-center justify-between">
                    <Text variant="large" numberOfLines={1} className="flex-1 pr-2">
                      {item.title || "Untitled chat"}
                    </Text>
                    <Badge variant="secondary">{relativeTime(item.updated_at)}</Badge>
                  </View>
                  {otherUser ? (
                    <Text variant="muted" className="mt-1" numberOfLines={1}>
                      Member: {item.user_id.slice(0, 8)}…
                    </Text>
                  ) : null}
                </CardContent>
              </Card>
            </Pressable>
          );
        }}
      />
    </View>
  );
}
