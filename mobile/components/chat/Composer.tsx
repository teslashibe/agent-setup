import { useState } from "react";
import {
  ActivityIndicator,
  Image,
  Pressable,
  ScrollView,
  TextInput,
  View
} from "react-native";
import { ImagePlus, SendHorizontal, X } from "lucide-react-native";

import { cn } from "@/lib/utils";
import { Text } from "@/components/ui/Text";

// Composer is the input dock at the bottom of the chat screen. The
// previous version was functional but flat: a small line-height
// TextInput, a square paperclip, a square send. The redesign:
//
//  - Bigger touch targets (40px round buttons vs 36px squares).
//  - Subtle focus ring on the surrounding surface so the operator
//    sees that typing has captured input.
//  - The send button becomes the *primary* affordance: bright
//    primary tint when the message is ready to ship; muted
//    secondary when the input is empty so it stops shouting at the
//    operator.
//  - A horizontal attachment strip lives ABOVE the textarea so
//    photos don't push the text off-screen as more get added.
//  - Disabled state is clearly indicated (cursor stays visible, but
//    the surface dims) when a turn is streaming.
//
// All file-upload state, picker logic, and submit lifecycle live in
// the parent screen — this component is presentation-only.

export type ComposerAttachment = {
  key: string;
  localUri: string;
  status: "uploading" | "ready" | "error";
};

type Props = {
  draft: string;
  onChangeDraft: (text: string) => void;

  attachments: ComposerAttachment[];
  onPickImage: () => void;
  onRemoveAttachment: (key: string) => void;
  picking: boolean;

  onSend: () => void;
  // True while a SSE stream is active. Disables the input + dims the
  // send button — the parent header surfaces the actual "Stop"
  // affordance, so we don't double up here.
  running: boolean;
  // True when one or more attachments are still mid-upload. Send is
  // blocked until all uploads are settled (success or removed).
  hasUploadingAttachment: boolean;
  // True when there's neither typed text nor any settled attachment
  // — i.e. there's nothing to send yet.
  empty: boolean;
};

export function Composer({
  draft,
  onChangeDraft,
  attachments,
  onPickImage,
  onRemoveAttachment,
  picking,
  onSend,
  running,
  hasUploadingAttachment,
  empty
}: Props) {
  const [focused, setFocused] = useState(false);
  const sendDisabled = running || hasUploadingAttachment || empty;
  const hasErrorAttachment = attachments.some((a) => a.status === "error");

  return (
    <View className="border-t border-border bg-background px-3 pb-3 pt-2">
      {attachments.length > 0 ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ gap: 8, paddingBottom: 8, paddingTop: 4 }}
          className="mb-1"
        >
          {attachments.map((att) => (
            <Thumbnail
              key={att.key}
              attachment={att}
              onRemove={() => onRemoveAttachment(att.key)}
            />
          ))}
        </ScrollView>
      ) : null}

      {hasErrorAttachment ? (
        <Text variant="muted" className="mb-1 px-1 text-xs text-destructive">
          One or more attachments failed to upload. Tap × to remove and try again.
        </Text>
      ) : null}

      <View
        className={cn(
          "flex-row items-end gap-2 rounded-3xl border bg-card px-2 py-1.5",
          focused ? "border-primary/60" : "border-border"
        )}
      >
        <Pressable
          onPress={onPickImage}
          disabled={picking || running}
          hitSlop={6}
          className={cn(
            "h-9 w-9 items-center justify-center rounded-full",
            "active:bg-secondary disabled:opacity-40"
          )}
          accessibilityLabel="Attach image"
        >
          {picking ? (
            <ActivityIndicator size="small" color="#9AA4B2" />
          ) : (
            <ImagePlus size={18} color="#9AA4B2" />
          )}
        </Pressable>

        <TextInput
          className="flex-1 max-h-40 px-1 py-2 text-base text-foreground"
          placeholder="Message your agent…"
          placeholderTextColor="#9AA4B2"
          multiline
          value={draft}
          onChangeText={onChangeDraft}
          onSubmitEditing={onSend}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          editable={!running}
          // RN-web sometimes resets line-height to a value that
          // clips descenders in the dark theme; explicit lineHeight
          // matches the iOS/Android baseline.
          style={{ lineHeight: 20 }}
        />

        <Pressable
          onPress={onSend}
          disabled={sendDisabled}
          className={cn(
            "h-9 w-9 items-center justify-center rounded-full",
            sendDisabled ? "bg-secondary" : "bg-primary active:opacity-90"
          )}
          accessibilityLabel="Send message"
        >
          {running ? (
            <ActivityIndicator size="small" color="#9AA4B2" />
          ) : (
            <SendHorizontal
              size={16}
              color={sendDisabled ? "#9AA4B2" : "#06070A"}
            />
          )}
        </Pressable>
      </View>
    </View>
  );
}

// Thumbnail is the in-composer chip for one picked image. While the
// upload is in flight we overlay a spinner; on failure we tint the
// thumbnail red. The X always removes the chip from the draft (the
// upload itself can't be canceled mid-flight, but the chip going
// away means it'll never get into the message).
function Thumbnail({
  attachment,
  onRemove
}: {
  attachment: ComposerAttachment;
  onRemove: () => void;
}) {
  return (
    <View className="relative">
      <Image
        source={{ uri: attachment.localUri }}
        style={{
          width: 60,
          height: 60,
          borderRadius: 12,
          backgroundColor: "#0F141C"
        }}
        resizeMode="cover"
      />
      {attachment.status === "uploading" ? (
        <View
          className="absolute inset-0 items-center justify-center rounded-xl"
          style={{ backgroundColor: "rgba(6,7,10,0.55)" }}
        >
          <ActivityIndicator size="small" color="#00D4AA" />
        </View>
      ) : null}
      {attachment.status === "error" ? (
        <View
          className="absolute inset-0 items-center justify-center rounded-xl border border-destructive"
          style={{ backgroundColor: "rgba(255,90,103,0.25)" }}
        >
          <Text
            variant="muted"
            className="text-[10px] font-semibold text-destructive"
          >
            FAILED
          </Text>
        </View>
      ) : null}
      <Pressable
        onPress={onRemove}
        hitSlop={6}
        className="absolute -right-1.5 -top-1.5 h-5 w-5 items-center justify-center rounded-full border border-border bg-foreground"
        accessibilityLabel="Remove attachment"
      >
        <X size={12} color="#06070A" />
      </Pressable>
    </View>
  );
}
