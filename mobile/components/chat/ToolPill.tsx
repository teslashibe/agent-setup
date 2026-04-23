import { useEffect, useMemo, useRef, useState } from "react";
import { Animated, Easing, Platform, Pressable, View } from "react-native";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Loader2
} from "lucide-react-native";
import { useRouter } from "expo-router";

import { cn } from "@/lib/utils";
import { Text } from "@/components/ui/Text";
import { Button } from "@/components/ui/Button";
import { ToolInputPreview, ToolResult, decodeMcpOutput } from "@/components/chat/ToolResults";

// ToolPill is the new presentation for an MCP tool call inside an
// assistant turn. It deliberately replaces the loud full-width
// `Card`-based `ToolCard`: tool calls are *side effects* of the
// agent's reasoning, not first-class messages, so they should look
// like compact footnotes the operator can drill into when curious.
//
// Visual states:
//
//   ⏳  reddit_subreddit_about · running…       (animated spinner)
//   ✓   reddit_subreddit_about · done           (subtle, expandable)
//   !   reddit_submit · failed                  (destructive tint)
//
// Tap the pill to expand: shows the typed input pre-call, then the
// rich, tool-specific result renderer (or a structured error +
// reconnect CTA when the failure is a credential issue).
//
// Errors auto-expand on first render so the operator sees what went
// wrong without having to tap. Successful calls stay collapsed
// because the most informative thing in the chat is the agent's
// *next* text turn — the tool call itself is plumbing.

export type ToolPillCall = {
  id: string;
  name: string;
  input?: unknown;
  output?: unknown;
  isError?: boolean;
  done: boolean;
};

export function ToolPill({ tool }: { tool: ToolPillCall }) {
  const decoded = useMemo(
    () => (tool.done ? decodeMcpOutput(tool.output) : undefined),
    [tool.done, tool.output]
  );
  const credIssue = tool.isError ? detectCredentialIssue(decoded ?? tool.output) : null;
  // Auto-expand failures so the operator gets a heads-up without
  // having to tap; successes stay tucked away.
  const [open, setOpen] = useState<boolean>(!!tool.isError);

  const status: PillStatus = tool.isError ? "error" : tool.done ? "done" : "running";

  return (
    <View className="gap-1">
      <Pressable
        onPress={() => setOpen((o) => !o)}
        hitSlop={6}
        className={cn(
          "flex-row items-center gap-1.5 self-start rounded-full border px-2.5 py-1",
          statusBg[status]
        )}
        accessibilityRole="button"
        accessibilityLabel={`${tool.name} tool call, ${status}, tap to ${open ? "collapse" : "expand"}`}
      >
        <StatusIcon status={status} />
        <Text variant="small" className={cn("text-xs font-medium", statusText[status])}>
          {humanizeToolName(tool.name)}
        </Text>
        <Text variant="muted" className={cn("text-xs", statusSubtle[status])}>
          {statusLabel[status]}
        </Text>
        <View className="ml-1">
          {open ? (
            <ChevronDown size={12} color={statusIconColor[status]} />
          ) : (
            <ChevronRight size={12} color={statusIconColor[status]} />
          )}
        </View>
      </Pressable>

      {open ? (
        <View className="ml-3 gap-2 rounded-xl border border-border bg-card/60 px-3 py-2">
          {tool.input !== undefined ? (
            <View className="gap-1">
              <Text variant="muted" className="text-[10px] uppercase tracking-wider">
                Input
              </Text>
              <ToolInputPreview input={tool.input} />
            </View>
          ) : null}

          {tool.done ? (
            tool.isError ? (
              <View className="gap-1">
                <Text variant="muted" className="text-[10px] uppercase tracking-wider">
                  Error
                </Text>
                <Text variant="small" className="text-xs text-destructive">
                  {extractErrorText(decoded ?? tool.output)}
                </Text>
                {credIssue ? (
                  <ReconnectCTA platform={credIssue.platform} reason={credIssue.reason} />
                ) : null}
              </View>
            ) : (
              <View className="gap-1">
                <Text variant="muted" className="text-[10px] uppercase tracking-wider">
                  Result
                </Text>
                <ToolResult toolName={tool.name} output={tool.output} />
              </View>
            )
          ) : null}
        </View>
      ) : null}
    </View>
  );
}

// humanizeToolName turns `reddit_subreddit_about` into
// `Reddit · Subreddit about` — short enough to fit in a pill, but
// readable. We split on `_` rather than naming each tool because the
// MCP surface grows; this gives reasonable defaults for free.
function humanizeToolName(name: string): string {
  const parts = name.split("_");
  if (parts.length < 2) return prettyWord(name);
  const platform = prettyWord(parts[0]);
  const action = parts.slice(1).join(" ");
  return `${platform} · ${prettyWord(action)}`;
}

function prettyWord(s: string): string {
  if (!s) return s;
  return s[0].toUpperCase() + s.slice(1);
}

type PillStatus = "running" | "done" | "error";

const statusBg: Record<PillStatus, string> = {
  running: "border-primary/30 bg-primary/10",
  done: "border-border bg-secondary",
  error: "border-destructive/40 bg-destructive/10"
};
const statusText: Record<PillStatus, string> = {
  running: "text-primary",
  done: "text-foreground",
  error: "text-destructive"
};
const statusSubtle: Record<PillStatus, string> = {
  running: "text-primary/70",
  done: "text-muted",
  error: "text-destructive/80"
};
const statusLabel: Record<PillStatus, string> = {
  running: "running…",
  done: "done",
  error: "error"
};
const statusIconColor: Record<PillStatus, string> = {
  running: "#00D4AA",
  done: "#9AA4B2",
  error: "#FF5A67"
};

function StatusIcon({ status }: { status: PillStatus }) {
  if (status === "done") return <CheckCircle2 size={12} color={statusIconColor.done} />;
  if (status === "error") return <AlertTriangle size={12} color={statusIconColor.error} />;
  return <Spinner />;
}

// Spinner uses the lucide Loader2 glyph and an Animated rotation
// loop so the pill visibly "works" while the tool call is in
// flight. We use the native driver where available (transform/opacity
// are both compatible) so the animation stays smooth even when the
// JS thread is busy parsing SSE chunks.
function Spinner() {
  const spin = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    const loop = Animated.loop(
      Animated.timing(spin, {
        toValue: 1,
        duration: 900,
        easing: Easing.linear,
        // RN-web has no native animation module, so platform-gate to
        // avoid the one-time fallback warning.
        useNativeDriver: Platform.OS !== "web"
      })
    );
    loop.start();
    return () => loop.stop();
  }, [spin]);
  const rotate = spin.interpolate({ inputRange: [0, 1], outputRange: ["0deg", "360deg"] });
  return (
    <Animated.View style={{ transform: [{ rotate }] }}>
      <Loader2 size={12} color={statusIconColor.running} />
    </Animated.View>
  );
}

function extractErrorText(output: unknown): string {
  if (typeof output === "string") return output;
  if (output && typeof output === "object") {
    const msg =
      (output as any).message ?? (output as any).error ?? (output as any).text;
    if (typeof msg === "string") return msg;
    try {
      return JSON.stringify(output);
    } catch {
      return String(output);
    }
  }
  return "(no error message)";
}

const CREDENTIAL_ERROR_CODES = new Set([
  "credential_missing",
  "credential_invalid",
  "credential_expired",
  "credential_unreadable"
]);

/** When an MCP tool returns a structured credential error, surface a
 * "Reconnect" CTA inline with the failed tool pill. The MCP server
 * (`internal/mcp/server.go → mapErr`) emits errors of the shape:
 *   { code: "credential_expired", platform: "linkedin", retryable: true }
 * possibly wrapped inside a JSON-RPC `data` field. We probe several
 * shapes so we degrade gracefully if the wire format evolves. */
function detectCredentialIssue(
  output: unknown
): { platform: string; reason: string } | null {
  if (!output || typeof output !== "object") return null;
  const candidates: any[] = [output as any];
  if ((output as any).data) candidates.push((output as any).data);
  if (typeof (output as any).text === "string") {
    try {
      candidates.push(JSON.parse((output as any).text));
    } catch {
      /* not JSON — give up */
    }
  }
  for (const c of candidates) {
    if (!c || typeof c !== "object") continue;
    const code = String(c.code ?? c.error ?? "");
    if (CREDENTIAL_ERROR_CODES.has(code) && typeof c.platform === "string" && c.platform) {
      return { platform: c.platform, reason: String(c.message ?? code) };
    }
  }
  return null;
}

function ReconnectCTA({ platform, reason }: { platform: string; reason: string }) {
  const router = useRouter();
  return (
    <View className="mt-1 gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-2">
      <Text variant="small" className="text-destructive">
        {platform} credentials need re-connecting: {reason}
      </Text>
      <Button
        size="sm"
        variant="destructive"
        onPress={() => router.push("/(app)/platforms")}
      >
        Reconnect {platform}
      </Button>
    </View>
  );
}
