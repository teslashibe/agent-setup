import type { ComponentType } from "react";
import { Pressable, View } from "react-native";
import { ArrowLeft, Square } from "lucide-react-native";

import { Text } from "@/components/ui/Text";
import { brand } from "@/branding";

// ChatHeader is the sticky top bar of the chat screen. It replaces
// the previous header that just truncated the first user message
// into the title slot — that always read like the *prompt*, not the
// *agent*, and gave new operators no sense of who they were talking
// to or what the agent could do.
//
// New layout:
//
//   [<-]  [glyph]  Agent Name              [Stop]
//                 short subtitle · live
//
// The "Stop" button is only present while a turn is streaming. We
// surface it in the header (instead of replacing the send button in
// the composer) so the operator can interrupt without losing their
// in-progress draft, and so the affordance is visible even when the
// keyboard is up and the composer is partially hidden.
//
// All copy + the avatar glyph come from `branding.ts`. Forks rebrand
// by editing that surface — they do NOT need to touch this file.

type Props = {
  // Override the brand-default agent name for this header instance.
  agentName?: string;
  // Override the brand-default subtitle for this header instance.
  subtitle?: string;
  // Override the brand-default avatar glyph for this header instance.
  AvatarIcon?: ComponentType<{ size: number; color: string }>;
  // True while a `runSession` SSE stream is in flight; reveals the
  // stop button and the "live" pulse next to the subtitle.
  isStreaming?: boolean;
  // Called when the operator taps "Back" — usually `router.back()`.
  onBack: () => void;
  // Called when the operator taps "Stop"; aborts the SSE stream.
  onStop?: () => void;
};

export function ChatHeader({
  agentName,
  subtitle,
  AvatarIcon,
  isStreaming = false,
  onBack,
  onStop
}: Props) {
  const Icon = AvatarIcon ?? brand.AvatarIcon;
  const name = agentName ?? brand.name;
  const sub = subtitle ?? brand.subtitle;

  return (
    <View className="flex-row items-center gap-3 border-b border-border bg-background/95 px-4 pb-3 pt-12">
      <Pressable
        onPress={onBack}
        hitSlop={12}
        className="h-8 w-8 items-center justify-center rounded-full active:bg-secondary"
        accessibilityLabel="Back"
      >
        <ArrowLeft size={20} color="#F8FAFC" />
      </Pressable>

      <View
        className="h-9 w-9 items-center justify-center rounded-full"
        style={{ backgroundColor: "rgba(0, 212, 170, 0.15)" }}
      >
        <Icon size={18} color="#00D4AA" />
      </View>

      <View className="flex-1 min-w-0">
        <Text variant="large" className="text-base font-semibold" numberOfLines={1}>
          {name}
        </Text>
        <View className="flex-row items-center gap-1.5">
          <Text variant="muted" className="text-xs" numberOfLines={1}>
            {sub}
          </Text>
          {isStreaming ? (
            <>
              <Text variant="muted" className="text-xs">·</Text>
              <View className="flex-row items-center gap-1">
                <View
                  className="h-1.5 w-1.5 rounded-full bg-primary"
                  style={{ opacity: 0.85 }}
                />
                <Text className="text-xs text-primary" variant="muted">
                  live
                </Text>
              </View>
            </>
          ) : null}
        </View>
      </View>

      {isStreaming && onStop ? (
        <Pressable
          onPress={onStop}
          hitSlop={8}
          className="flex-row items-center gap-1.5 rounded-full border border-destructive/40 bg-destructive/10 px-3 py-1.5 active:bg-destructive/20"
          accessibilityLabel="Stop generation"
        >
          <Square size={10} color="#FF5A67" fill="#FF5A67" />
          <Text variant="small" className="text-xs font-medium text-destructive">
            Stop
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}
