import { useCallback, useState } from "react";
import { Alert, Pressable, ScrollView, View } from "react-native";
import { useRouter } from "expo-router";
import { ArrowLeft } from "lucide-react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Text } from "@/components/ui/Text";
import { useTeams } from "@/providers/TeamsProvider";
import { createTeam } from "@/services/teams";

// Dedicated route per audit M4 — splitting create-team out of the list lets
// the form get its own URL (deep-linkable from web), its own back-stack
// entry, and avoids the awkward in-list collapsible card.
export default function NewTeamScreen() {
  const router = useRouter();
  const { refresh, setActive } = useTeams();
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);

  const handleCreate = useCallback(async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      Alert.alert("Name required", "Give your team a name.");
      return;
    }
    setCreating(true);
    try {
      const created = await createTeam(trimmed);
      await refresh();
      // Switching active team is almost always what the user wants right
      // after creating it; saves a manual switcher tap.
      setActive(created.team.id);
      router.replace(`/(app)/teams/${created.team.id}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create team";
      Alert.alert("Create failed", message);
    } finally {
      setCreating(false);
    }
  }, [name, refresh, router, setActive]);

  return (
    <ScrollView
      className="flex-1 bg-background"
      contentContainerStyle={{ padding: 20, paddingTop: 48, paddingBottom: 140 }}
    >
      <Pressable className="mb-3 flex-row items-center gap-2" onPress={() => router.back()}>
        <ArrowLeft size={16} color="#9AA4B2" />
        <Text variant="muted">Back</Text>
      </Pressable>

      <Text variant="h2" className="mb-1">
        Create a team
      </Text>
      <Text variant="muted" className="mb-5">
        You'll be the owner. You can invite teammates after creation.
      </Text>

      <Card>
        <CardHeader>
          <CardTitle>Team details</CardTitle>
        </CardHeader>
        <CardContent>
          <Input
            label="Team name"
            value={name}
            onChangeText={setName}
            placeholder="e.g. Acme Engineering"
            autoFocus
          />
          <Button loading={creating} onPress={handleCreate}>
            Create team
          </Button>
        </CardContent>
      </Card>
    </ScrollView>
  );
}
