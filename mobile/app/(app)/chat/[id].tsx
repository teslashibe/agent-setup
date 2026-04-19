import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  TextInput,
  View
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { ArrowLeft, Send, Wrench } from "lucide-react-native";

import { Badge } from "@/components/ui/Badge";
import { Card, CardContent } from "@/components/ui/Card";
import { Text } from "@/components/ui/Text";
import { listMessages, runSession, type AgentEvent, type Message } from "@/services/agent";

type ChatBubble =
  | { kind: "user"; id: string; text: string }
  | { kind: "assistant"; id: string; text: string; tools: ToolCall[]; pending: boolean };

type ToolCall = {
  id: string;
  name: string;
  input?: unknown;
  output?: unknown;
  isError?: boolean;
  done: boolean;
};

// Anthropic content blocks have a discriminated `type`. We extract just enough
// to render the conversation history we already persisted.
type AnthropicBlock =
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input?: unknown }
  | { type: "tool_result"; tool_use_id: string; content: unknown; is_error?: boolean };

function bubblesFromHistory(messages: Message[]): ChatBubble[] {
  const out: ChatBubble[] = [];
  const toolCalls = new Map<string, ToolCall>();

  for (const m of messages) {
    const blocks = Array.isArray(m.content) ? (m.content as AnthropicBlock[]) : [];

    if (m.role === "user") {
      const text = blocks
        .filter((b): b is Extract<AnthropicBlock, { type: "text" }> => b.type === "text")
        .map((b) => b.text)
        .join("\n");
      const toolResults = blocks.filter(
        (b): b is Extract<AnthropicBlock, { type: "tool_result" }> => b.type === "tool_result"
      );
      for (const tr of toolResults) {
        const existing = toolCalls.get(tr.tool_use_id);
        if (existing) {
          existing.output = tr.content;
          existing.isError = tr.is_error;
          existing.done = true;
        }
      }
      if (text.trim().length > 0) {
        out.push({ kind: "user", id: m.id, text });
      }
      continue;
    }

    let text = "";
    const tools: ToolCall[] = [];
    for (const block of blocks) {
      if (block.type === "text") {
        text += (text ? "\n" : "") + block.text;
      } else if (block.type === "tool_use") {
        const tc: ToolCall = {
          id: block.id,
          name: block.name,
          input: block.input,
          done: false
        };
        tools.push(tc);
        toolCalls.set(block.id, tc);
      }
    }
    out.push({ kind: "assistant", id: m.id, text, tools, pending: false });
  }

  return out;
}

export default function ChatScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const sessionId = id!;

  const [bubbles, setBubbles] = useState<ChatBubble[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const scrollRef = useRef<ScrollView>(null);
  const abortRef = useRef<AbortController | null>(null);

  const scrollToEnd = useCallback(() => {
    requestAnimationFrame(() => scrollRef.current?.scrollToEnd({ animated: true }));
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const history = await listMessages(sessionId);
        if (cancelled) return;
        setBubbles(bubblesFromHistory(history));
      } catch (error) {
        const message = error instanceof Error ? error.message : "Failed to load messages";
        Alert.alert("Error", message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  useEffect(() => () => abortRef.current?.abort(), []);

  const send = useCallback(async () => {
    const text = draft.trim();
    if (!text || running) return;

    setDraft("");
    const userBubble: ChatBubble = { kind: "user", id: `local-${Date.now()}`, text };
    const assistantBubble: ChatBubble = {
      kind: "assistant",
      id: `local-${Date.now()}-a`,
      text: "",
      tools: [],
      pending: true
    };
    setBubbles((prev) => [...prev, userBubble, assistantBubble]);
    scrollToEnd();
    setRunning(true);

    const controller = new AbortController();
    abortRef.current = controller;

    const updateAssistant = (mutator: (b: Extract<ChatBubble, { kind: "assistant" }>) => void) => {
      setBubbles((prev) => {
        const next = [...prev];
        const idx = next.findIndex((b) => b.id === assistantBubble.id);
        if (idx === -1) return prev;
        const target = next[idx];
        if (target.kind !== "assistant") return prev;
        const updated = { ...target, tools: target.tools.map((t) => ({ ...t })) };
        mutator(updated);
        next[idx] = updated;
        return next;
      });
    };

    try {
      for await (const ev of runSession(sessionId, text, controller.signal)) {
        applyEvent(ev, updateAssistant);
        scrollToEnd();
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "Stream failed";
      updateAssistant((b) => {
        b.pending = false;
        b.text = b.text || `[error] ${message}`;
      });
    } finally {
      updateAssistant((b) => {
        b.pending = false;
      });
      setRunning(false);
      abortRef.current = null;
    }
  }, [draft, running, scrollToEnd, sessionId]);

  const headerTitle = useMemo(() => {
    const firstUser = bubbles.find((b) => b.kind === "user") as Extract<ChatBubble, { kind: "user" }> | undefined;
    return firstUser?.text.slice(0, 40) ?? "Chat";
  }, [bubbles]);

  if (loading) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-background"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={0}
    >
      <View className="flex-row items-center gap-3 px-5 pt-12 pb-3 border-b border-border">
        <Pressable onPress={() => router.back()} hitSlop={12}>
          <ArrowLeft size={22} color="#F8FAFC" />
        </Pressable>
        <Text variant="large" numberOfLines={1} className="flex-1">
          {headerTitle}
        </Text>
      </View>

      <ScrollView
        ref={scrollRef}
        className="flex-1"
        contentContainerStyle={{ padding: 16, gap: 12, paddingBottom: 24 }}
        onContentSizeChange={scrollToEnd}
      >
        {bubbles.map((bubble) => (
          <Bubble key={bubble.id} bubble={bubble} />
        ))}
      </ScrollView>

      <View className="border-t border-border bg-background px-3 py-3">
        <View className="flex-row items-end gap-2 rounded-2xl border border-border bg-card px-3 py-2">
          <TextInput
            className="flex-1 max-h-32 text-foreground"
            placeholder="Message your agent…"
            placeholderTextColor="#9AA4B2"
            multiline
            value={draft}
            onChangeText={setDraft}
            onSubmitEditing={send}
            editable={!running}
          />
          <Pressable
            onPress={send}
            disabled={running || draft.trim().length === 0}
            className="h-9 w-9 items-center justify-center rounded-xl bg-primary disabled:opacity-50"
          >
            {running ? (
              <ActivityIndicator size="small" color="#06070A" />
            ) : (
              <Send size={16} color="#06070A" />
            )}
          </Pressable>
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}

function applyEvent(
  ev: AgentEvent,
  updateAssistant: (mutator: (b: Extract<ChatBubble, { kind: "assistant" }>) => void) => void
) {
  switch (ev.type) {
    case "text":
      updateAssistant((b) => {
        b.text += ev.text;
      });
      return;
    case "tool_use":
      updateAssistant((b) => {
        b.tools.push({ id: ev.tool_id, name: ev.tool, input: ev.input, done: false });
      });
      return;
    case "tool_result":
      updateAssistant((b) => {
        const tool = b.tools.find((t) => t.id === ev.tool_id);
        if (tool) {
          tool.output = ev.output;
          tool.isError = ev.is_error;
          tool.done = true;
        }
      });
      return;
    case "done":
      updateAssistant((b) => {
        b.pending = false;
      });
      return;
    case "error":
      updateAssistant((b) => {
        b.text += (b.text ? "\n" : "") + `[error] ${ev.error}`;
        b.pending = false;
      });
      return;
    case "usage":
    default:
      return;
  }
}

function Bubble({ bubble }: { bubble: ChatBubble }) {
  if (bubble.kind === "user") {
    return (
      <View className="self-end max-w-[85%] rounded-2xl bg-primary px-4 py-2">
        <Text variant="small" className="text-background">
          {bubble.text}
        </Text>
      </View>
    );
  }

  const showCursor = bubble.pending && bubble.text.length === 0 && bubble.tools.length === 0;

  return (
    <View className="self-start max-w-[90%] gap-2">
      {bubble.tools.map((tool) => (
        <Card key={tool.id}>
          <CardContent className="gap-1">
            <View className="flex-row items-center gap-2">
              <Wrench size={14} color={tool.isError ? "#FF5A67" : "#00D4AA"} />
              <Text variant="small" className="font-semibold">
                {tool.name}
              </Text>
              <Badge variant={tool.isError ? "destructive" : tool.done ? "default" : "secondary"}>
                {tool.isError ? "error" : tool.done ? "done" : "running"}
              </Badge>
            </View>
            {tool.input !== undefined ? (
              <Text variant="muted" className="text-xs">
                input: {safeJson(tool.input)}
              </Text>
            ) : null}
            {tool.done ? (
              <Text variant="muted" className="text-xs">
                output: {safeJson(tool.output)}
              </Text>
            ) : null}
          </CardContent>
        </Card>
      ))}
      {bubble.text.length > 0 || showCursor ? (
        <View className="rounded-2xl border border-border bg-card px-4 py-2">
          <Text variant="small">
            {bubble.text}
            {bubble.pending ? " ▌" : ""}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

function safeJson(value: unknown): string {
  if (value === undefined) return "(none)";
  try {
    const s = JSON.stringify(value);
    return s.length > 240 ? s.slice(0, 240) + "…" : s;
  } catch {
    return String(value);
  }
}
