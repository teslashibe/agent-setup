import { useState } from "react";
import { Pressable, View } from "react-native";
import * as Clipboard from "expo-clipboard";
import * as Haptics from "expo-haptics";
import { Check, Copy, User } from "lucide-react-native";

import { cn } from "@/lib/utils";
import { Text } from "@/components/ui/Text";
import { Markdown } from "@/components/chat/Markdown";
import { ToolPill, type ToolPillCall } from "@/components/chat/ToolPill";
import { StreamingDots } from "@/components/chat/StreamingDots";
import { brand } from "@/branding";

// MessageBubble renders a single chat turn — either the operator's
// outgoing message (right-aligned, primary tint, no avatar) or the
// agent's response (left-aligned with avatar, neutral surface, with
// a quiet action row underneath for "copy").
//
// The two roles share a row layout (avatar + bubble column) so we can
// vary just the alignment, surface color, and trailing chrome (e.g.
// the agent gets a copy button; the operator doesn't, because their
// own input is already at hand).
//
// Tool calls (`tools[]`) live with the assistant turn and render as
// compact pills *above* the prose. Putting them above makes the
// reading order match the agent's actual workflow ("I called this
// tool, then wrote this draft") and keeps the assistant prose as the
// stable visual anchor of the message.

export type AssistantBubbleData = {
  kind: "assistant";
  id: string;
  text: string;
  tools: ToolPillCall[];
  pending: boolean;
};

export type UserBubbleData = {
  kind: "user";
  id: string;
  text: string;
};

export function MessageBubble({ bubble }: { bubble: AssistantBubbleData | UserBubbleData }) {
  if (bubble.kind === "user") {
    return <UserMessage bubble={bubble} />;
  }
  return <AssistantMessage bubble={bubble} />;
}

function UserMessage({ bubble }: { bubble: UserBubbleData }) {
  return (
    <View className="flex-row justify-end">
      <View className="max-w-[85%] rounded-2xl rounded-br-md bg-primary px-4 py-2.5 shadow-sm">
        <Markdown content={bubble.text} tone="onAccent" />
      </View>
    </View>
  );
}

function AssistantMessage({ bubble }: { bubble: AssistantBubbleData }) {
  const showThinking =
    bubble.pending && bubble.text.length === 0 && bubble.tools.length === 0;
  const showWorking =
    bubble.pending && bubble.text.length === 0 && bubble.tools.length > 0;

  return (
    <View className="flex-row gap-2.5">
      <AgentAvatar />
      <View className="min-w-0 flex-1 gap-1.5">
        <View className="flex-row items-baseline gap-2">
          <Text variant="small" className="font-semibold text-foreground">
            {brand.name}
          </Text>
          {bubble.pending ? (
            <Text variant="muted" className="text-[10px] uppercase tracking-wider">
              live
            </Text>
          ) : null}
        </View>

        {bubble.tools.length > 0 ? (
          <View className="gap-1.5">
            {bubble.tools.map((tool) => (
              <ToolPill key={tool.id} tool={tool} />
            ))}
          </View>
        ) : null}

        {showThinking ? <StreamingDots label="Thinking" /> : null}
        {showWorking ? <StreamingDots label="Working" /> : null}

        {bubble.text.length > 0 ? (
          <View className="rounded-2xl rounded-tl-md border border-border bg-card px-4 py-3">
            <Markdown content={bubble.text} />
            {bubble.pending ? (
              <View className="mt-1">
                <StreamingDots silent />
              </View>
            ) : null}
          </View>
        ) : null}

        {!bubble.pending && bubble.text.length > 0 ? (
          <MessageActions text={bubble.text} />
        ) : null}
      </View>
    </View>
  );
}

// AgentAvatar is the small circular brand mark to the left of every
// agent message. The glyph + tint come from `branding.ts` so forks
// rebrand once and the whole chat picks it up.
function AgentAvatar() {
  const Icon = brand.AvatarIcon;
  return (
    <View
      className="h-8 w-8 items-center justify-center rounded-full"
      style={{ backgroundColor: "rgba(0, 212, 170, 0.15)" }}
    >
      <Icon size={16} color="#00D4AA" />
    </View>
  );
}

// UserAvatar is unused right now (operator messages stay
// avatar-less to keep the chat feeling like a focused tool, not a
// social DM). Exported in case a future "all sessions" admin view
// wants to render it.
export function UserAvatar() {
  return (
    <View className="h-8 w-8 items-center justify-center rounded-full bg-secondary">
      <User size={16} color="#9AA4B2" />
    </View>
  );
}

// MessageActions is the small row underneath every settled assistant
// turn. Right now it's just "Copy" — the agent's drafts are the
// primary thing operators want to reuse (dropping into a different
// editor, scheduling tools, etc.). Future buttons live here too:
// "Regenerate", "Save as template", "Reply with quote".
function MessageActions({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    try {
      await Clipboard.setStringAsync(text);
      setCopied(true);
      void Haptics.selectionAsync().catch(() => undefined);
      // Auto-revert the icon after a beat so the operator can copy
      // again without the affordance feeling sticky.
      setTimeout(() => setCopied(false), 1400);
    } catch {
      /* clipboard write blocked — silently no-op */
    }
  };
  return (
    <View className="flex-row items-center gap-1">
      <Pressable
        onPress={onCopy}
        hitSlop={6}
        className={cn(
          "flex-row items-center gap-1 rounded-md px-2 py-1",
          copied ? "bg-primary/10" : "active:bg-secondary"
        )}
        accessibilityLabel={copied ? "Copied" : "Copy message"}
      >
        {copied ? (
          <Check size={12} color="#00D4AA" />
        ) : (
          <Copy size={12} color="#9AA4B2" />
        )}
        <Text variant="muted" className={cn("text-[11px]", copied && "text-primary")}>
          {copied ? "Copied" : "Copy"}
        </Text>
      </Pressable>
    </View>
  );
}
