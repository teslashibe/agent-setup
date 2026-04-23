import { useCallback, useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
  Platform,
  ScrollView,
  View
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import * as ImagePicker from "expo-image-picker";

import { ChatEmptyState } from "@/components/chat/ChatEmptyState";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { Composer, type ComposerAttachment } from "@/components/chat/Composer";
import {
  MessageBubble,
  type AssistantBubbleData,
  type UserBubbleData
} from "@/components/chat/MessageBubble";
import { ScrollToLatest } from "@/components/chat/ScrollToLatest";
import type { ToolPillCall } from "@/components/chat/ToolPill";
import { listMessages, runSession, type AgentEvent, type Message } from "@/services/agent";
import { uploadAttachment, type UploadedAttachment } from "@/services/uploads";

type ChatBubble = UserBubbleData | AssistantBubbleData;

// Anthropic content blocks have a discriminated `type`. We extract
// just enough to render the conversation history we already
// persisted.
type AnthropicBlock =
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input?: unknown }
  | { type: "tool_result"; tool_use_id: string; content: unknown; is_error?: boolean };

// bubblesFromHistory translates the persisted Anthropic-format
// message log into the flatter `ChatBubble` array the UI renders.
//
// The two subtle bits of logic:
//
//   1. tool_result blocks live in user-role messages (Anthropic
//      convention). We thread them back into the assistant bubble
//      that issued the corresponding tool_use by tracking a
//      `tool_use_id → ToolCall` map.
//
//   2. After the walk, any tool call still flagged `done: false`
//      while there are subsequent messages gets force-marked done.
//      This catches the "I reloaded mid-conversation" case where the
//      tool_result was persisted but didn't get matched (e.g. an
//      older message version, or an out-of-order persistence quirk).
//      The downside — a genuinely-still-running tool would be marked
//      done — is acceptable because by the time the operator is
//      viewing persisted history, no tools are actually still in
//      flight.
function bubblesFromHistory(messages: Message[]): ChatBubble[] {
  const out: ChatBubble[] = [];
  const toolCalls = new Map<string, ToolPillCall>();

  // Stable per-row id so React keys are guaranteed unique even if
  // the persisted message id is missing or duplicated. Using the
  // array index as a suffix is fine because history is read once
  // and re-renders use the same indexes.
  const rowId = (m: Message, mi: number) =>
    m.id ? `${m.id}` : `m-${mi}`;

  for (let mi = 0; mi < messages.length; mi++) {
    const m = messages[mi];
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
        out.push({ kind: "user", id: rowId(m, mi), text });
      }
      continue;
    }

    let text = "";
    const tools: ToolPillCall[] = [];
    for (const block of blocks) {
      if (block.type === "text") {
        text += (text ? "\n" : "") + block.text;
      } else if (block.type === "tool_use") {
        const tc: ToolPillCall = {
          id: block.id,
          name: block.name,
          input: block.input,
          done: false
        };
        tools.push(tc);
        toolCalls.set(block.id, tc);
      }
    }
    out.push({ kind: "assistant", id: rowId(m, mi), text, tools, pending: false });
  }

  // Defensive sweep: in persisted history every tool call has
  // already settled — the agent couldn't have produced its written
  // response (and we wouldn't have persisted the message) without
  // the tool result coming back. The earlier walk only flips `done`
  // when it can pair the tool_use with a tool_result block; if the
  // pairing fell through (older message format, the agent never
  // wrote anything after the call, etc.) we mark it done here so
  // the pill renders "done" instead of a forever-spinning
  // "running…".
  for (const b of out) {
    if (b.kind !== "assistant") continue;
    for (const tool of b.tools) {
      if (!tool.done) tool.done = true;
    }
  }

  return out;
}

// PendingAttachment is a single image the operator has attached to
// the in-progress draft. We hold both the local picker URI (for
// showing a thumbnail before the upload completes) and, once the
// `/api/uploads` round-trip lands, the signed URL we'll embed in the
// outgoing message and the agent's MCP tool will fetch.
type PendingAttachment = {
  key: string;
  localUri: string;
  fileName?: string;
  mimeType?: string;
  size?: number;
  status: "uploading" | "ready" | "error";
  uploaded?: UploadedAttachment;
  errorMessage?: string;
};

// Mirror the backend's MaxBytes (10 MiB). Catching this client-side
// keeps us from streaming a 12 MB photo just to get a 413 back.
const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024;

// Distance (in px) from the bottom of the scroll content under
// which we count the user as "near the bottom" — used both to
// decide whether to autoscroll on new content and whether to show
// the "jump to latest" button.
const NEAR_BOTTOM_THRESHOLD = 80;

export default function ChatScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const sessionId = id!;

  const [bubbles, setBubbles] = useState<ChatBubble[]>([]);
  const [draft, setDraft] = useState("");
  const [attachments, setAttachments] = useState<PendingAttachment[]>([]);
  const [picking, setPicking] = useState(false);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [nearBottom, setNearBottom] = useState(true);
  const [hasUnseenStreamUpdate, setHasUnseenStreamUpdate] = useState(false);

  const scrollRef = useRef<ScrollView>(null);
  const abortRef = useRef<AbortController | null>(null);
  const nearBottomRef = useRef(true);

  const scrollToEnd = useCallback((animated = true) => {
    requestAnimationFrame(() => scrollRef.current?.scrollToEnd({ animated }));
  }, []);

  // Auto-scroll only if the user is already pinned to the bottom.
  // Otherwise we'd yank them out of mid-history reading every time a
  // streamed token arrived.
  const maybeAutoScroll = useCallback(() => {
    if (nearBottomRef.current) {
      scrollToEnd();
    } else {
      setHasUnseenStreamUpdate(true);
    }
  }, [scrollToEnd]);

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

  const handleScroll = useCallback((ev: NativeSyntheticEvent<NativeScrollEvent>) => {
    const { contentOffset, contentSize, layoutMeasurement } = ev.nativeEvent;
    const distanceFromBottom =
      contentSize.height - (contentOffset.y + layoutMeasurement.height);
    const next = distanceFromBottom < NEAR_BOTTOM_THRESHOLD;
    nearBottomRef.current = next;
    setNearBottom(next);
    if (next) setHasUnseenStreamUpdate(false);
  }, []);

  const updateAttachment = useCallback(
    (key: string, mutator: (a: PendingAttachment) => PendingAttachment) => {
      setAttachments((prev) => prev.map((a) => (a.key === key ? mutator(a) : a)));
    },
    []
  );

  const removeAttachment = useCallback((key: string) => {
    setAttachments((prev) => prev.filter((a) => a.key !== key));
  }, []);

  const startUpload = useCallback(
    async (att: PendingAttachment) => {
      try {
        const uploaded = await uploadAttachment({
          uri: att.localUri,
          name: att.fileName,
          mimeType: att.mimeType
        });
        updateAttachment(att.key, (a) => ({ ...a, status: "ready", uploaded }));
      } catch (err) {
        const message = err instanceof Error ? err.message : "Upload failed";
        updateAttachment(att.key, (a) => ({ ...a, status: "error", errorMessage: message }));
      }
    },
    [updateAttachment]
  );

  const pickImage = useCallback(async () => {
    if (picking || running) return;
    setPicking(true);
    try {
      const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!perm.granted) {
        Alert.alert(
          "Photos access needed",
          "Allow photo library access in Settings to attach images to your messages."
        );
        return;
      }
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        allowsMultipleSelection: false,
        // Light compression — most platforms accept images up to
        // ~20MB and typical phone shots are 4-8MB at quality 0.85,
        // which keeps us comfortably under the backend's 10MB cap.
        quality: 0.85,
        exif: false
      });
      if (result.canceled || result.assets.length === 0) return;
      const asset = result.assets[0];
      if (asset.fileSize && asset.fileSize > MAX_ATTACHMENT_BYTES) {
        Alert.alert(
          "Image too large",
          "Pick an image under 10MB. Long-press the photo in Photos and re-export at a smaller size."
        );
        return;
      }
      const att: PendingAttachment = {
        key: `att-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        localUri: asset.uri,
        fileName: asset.fileName ?? undefined,
        mimeType: asset.mimeType ?? undefined,
        size: asset.fileSize ?? undefined,
        status: "uploading"
      };
      setAttachments((prev) => [...prev, att]);
      void startUpload(att);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Could not open photo library";
      Alert.alert("Picker error", message);
    } finally {
      setPicking(false);
    }
  }, [picking, running, startUpload]);

  const hasUploadingAttachment = attachments.some((a) => a.status === "uploading");
  const readyAttachments = attachments.filter(
    (a): a is PendingAttachment & { status: "ready"; uploaded: UploadedAttachment } =>
      a.status === "ready" && !!a.uploaded
  );

  const composerAttachments: ComposerAttachment[] = attachments.map((a) => ({
    key: a.key,
    localUri: a.localUri,
    status: a.status
  }));

  const send = useCallback(async () => {
    const text = draft.trim();
    if (running || hasUploadingAttachment) return;
    if (!text && readyAttachments.length === 0) return;

    // Compose the wire-format message: user text first (if any),
    // then a blank line, then one `![name](signed-url)` per
    // attachment. The agent's system prompt tells it to look for
    // `![…](https://…)` markers in the operator's turn and pass the
    // URL straight through to its image-aware tools.
    const imageBlock = readyAttachments
      .map((a) => `![${a.uploaded.original_name || "attached image"}](${a.uploaded.url})`)
      .join("\n");
    const wire = [text, imageBlock].filter((part) => part.length > 0).join("\n\n");

    setDraft("");
    setAttachments([]);
    const userBubble: ChatBubble = { kind: "user", id: `local-${Date.now()}`, text: wire };
    const assistantBubble: ChatBubble = {
      kind: "assistant",
      id: `local-${Date.now()}-a`,
      text: "",
      tools: [],
      pending: true
    };
    setBubbles((prev) => [...prev, userBubble, assistantBubble]);
    // Pin to bottom on send so the operator follows their own
    // turn — they almost always want to see what comes back.
    nearBottomRef.current = true;
    setHasUnseenStreamUpdate(false);
    scrollToEnd();
    setRunning(true);

    const controller = new AbortController();
    abortRef.current = controller;

    const updateAssistant = (mutator: (b: AssistantBubbleData) => void) => {
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
      for await (const ev of runSession(sessionId, wire, controller.signal)) {
        applyEvent(ev, updateAssistant);
        maybeAutoScroll();
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
  }, [
    draft,
    hasUploadingAttachment,
    maybeAutoScroll,
    readyAttachments,
    running,
    scrollToEnd,
    sessionId
  ]);

  const onStop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const onPickSuggestion = useCallback((prompt: string) => {
    setDraft(prompt);
  }, []);

  const onJumpToLatest = useCallback(() => {
    nearBottomRef.current = true;
    setHasUnseenStreamUpdate(false);
    scrollToEnd();
  }, [scrollToEnd]);

  if (loading) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  const empty = bubbles.length === 0;

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-background"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={0}
    >
      <ChatHeader
        isStreaming={running}
        onBack={() => router.back()}
        onStop={onStop}
      />

      <View className="relative flex-1">
        <ScrollView
          ref={scrollRef}
          className="flex-1"
          contentContainerStyle={{
            padding: 16,
            paddingBottom: 28,
            gap: 16,
            flexGrow: 1
          }}
          onContentSizeChange={() => {
            if (nearBottomRef.current) scrollToEnd(false);
          }}
          onScroll={handleScroll}
          scrollEventThrottle={32}
        >
          {empty ? (
            <ChatEmptyState onPick={onPickSuggestion} />
          ) : (
            bubbles.map((bubble, idx) => (
              // Defensive key: persisted message ids should be unique
              // UUIDs, but if the backend ever sends a row without an
              // id we fall back to the array index so React doesn't
              // throw a key-warning at us in dev.
              <MessageBubble
                key={bubble.id || `bubble-${idx}`}
                bubble={bubble}
              />
            ))
          )}
        </ScrollView>

        <ScrollToLatest
          visible={!empty && !nearBottom}
          hasNew={hasUnseenStreamUpdate}
          onPress={onJumpToLatest}
        />
      </View>

      <Composer
        draft={draft}
        onChangeDraft={setDraft}
        attachments={composerAttachments}
        onPickImage={pickImage}
        onRemoveAttachment={removeAttachment}
        picking={picking}
        onSend={send}
        running={running}
        hasUploadingAttachment={hasUploadingAttachment}
        empty={draft.trim().length === 0 && readyAttachments.length === 0}
      />
    </KeyboardAvoidingView>
  );
}

function applyEvent(
  ev: AgentEvent,
  updateAssistant: (mutator: (b: AssistantBubbleData) => void) => void
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
