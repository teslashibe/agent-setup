import { Pressable, View } from "react-native";
import { ArrowDown } from "lucide-react-native";

import { Text } from "@/components/ui/Text";

// ScrollToLatest is the floating "jump to bottom" button that
// appears when the operator has scrolled up away from the latest
// turn. Long agent drafts (especially with many tool pills) push the
// composer out of view; without this, the operator can lose track
// of where the streaming output is going.
//
// The button auto-hides when the operator is already near the
// bottom (parent decides via `visible`) so it doesn't sit on top of
// the composer in the common case.
//
// While the agent is streaming we also surface a tiny "new message"
// pulse — a soft primary tint behind the icon — so the operator
// notices that there's content waiting even if they're reading
// earlier in the thread.

type Props = {
  visible: boolean;
  hasNew?: boolean;
  onPress: () => void;
};

export function ScrollToLatest({ visible, hasNew = false, onPress }: Props) {
  if (!visible) return null;
  return (
    <View pointerEvents="box-none" className="absolute inset-x-0 bottom-2 items-center">
      <Pressable
        onPress={onPress}
        className="flex-row items-center gap-1.5 rounded-full border border-border bg-card/95 px-3 py-1.5 active:opacity-80"
        style={{
          shadowColor: "#000",
          shadowOpacity: 0.25,
          shadowOffset: { width: 0, height: 2 },
          shadowRadius: 8
        }}
        accessibilityLabel="Jump to latest message"
      >
        <ArrowDown size={14} color={hasNew ? "#00D4AA" : "#9AA4B2"} />
        <Text
          variant="small"
          className={`text-xs ${hasNew ? "text-primary font-semibold" : "text-muted"}`}
        >
          {hasNew ? "New message" : "Jump to latest"}
        </Text>
      </Pressable>
    </View>
  );
}
