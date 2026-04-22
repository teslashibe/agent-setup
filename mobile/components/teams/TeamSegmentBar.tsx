import { Pressable, View } from "react-native";
import { useRouter, useSegments } from "expo-router";

import { Text } from "@/components/ui/Text";
import { cn } from "@/lib/utils";

// TeamSegmentBar is the header strip that swaps between
//   /teams/:id (overview)  ·  /teams/:id/members  ·  /teams/:id/invites
//
// We keep it as a horizontal pill row instead of native Tabs so the parent
// Stack can keep its single back-button + the children stay simple route
// files (no nested navigator). showInvites is passed in by the parent so
// we hide the tab entirely for non-admins.
export function TeamSegmentBar({
  teamID,
  showInvites,
}: {
  teamID: string;
  showInvites: boolean;
}) {
  const router = useRouter();
  const segments = useSegments() as string[];
  // Last segment of the current route — "members", "invites", or undefined
  // when on the overview index. Compare against [id] is unreliable because
  // the dynamic key gets resolved to the actual id at runtime.
  const last = segments[segments.length - 1];
  const tabs: { id: "overview" | "members" | "invites"; label: string; route: string }[] = [
    { id: "overview", label: "Overview", route: `/(app)/teams/${teamID}` },
    { id: "members", label: "Members", route: `/(app)/teams/${teamID}/members` },
  ];
  if (showInvites) {
    tabs.push({ id: "invites", label: "Invites", route: `/(app)/teams/${teamID}/invites` });
  }
  // active falls back to "overview" because the index segment is the dynamic
  // [id] segment — there's no literal "overview" path on disk.
  const active =
    last === "members" ? "members" : last === "invites" ? "invites" : "overview";

  return (
    <View className="flex-row gap-1 rounded-lg border border-border bg-card p-1">
      {tabs.map((t) => {
        const isActive = active === t.id;
        return (
          <Pressable
            key={t.id}
            onPress={() => router.push(t.route as never)}
            className={cn(
              "flex-1 items-center justify-center rounded-md px-3 py-2",
              isActive ? "bg-primary/15" : "",
            )}
          >
            <Text
              variant="small"
              className={cn(
                "font-semibold",
                isActive ? "text-primary" : "text-foreground/80",
              )}
            >
              {t.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}
