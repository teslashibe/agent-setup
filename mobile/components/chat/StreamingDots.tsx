import { useEffect, useRef } from "react";
import { Animated, Easing, Platform, View } from "react-native";

import { Text } from "@/components/ui/Text";

// React Native web doesn't ship the native animation module, so
// `useNativeDriver: true` would log a one-time warning and fall back
// to JS-driven animation anyway. Platform-gating keeps the console
// clean without changing behavior on iOS/Android.
const USE_NATIVE_DRIVER = Platform.OS !== "web";

// StreamingDots is the "the agent is thinking" affordance shown
// inside an assistant bubble before any text has arrived. It
// replaces the previous "▌" trailing cursor, which was both visually
// inert and hard to distinguish from a stalled stream.
//
// Three muted dots fade in/out in sequence at ~360ms cadence — the
// same beat as iMessage / Claude.ai, so it reads as "actively
// working" without being attention-grabbing. We pair the dots with a
// faint label so first-time operators know what they're seeing
// instead of guessing.
//
// Driven entirely by Animated.opacity with `useNativeDriver: true`
// (on native) so the animation stays smooth even while the JS thread
// is busy parsing inbound SSE chunks.

type Props = {
  // Optional override label. The default ("Thinking") is right for
  // the very-start-of-turn case; pass "Working…" once the agent is
  // mid-tool-loop and we want to nudge that distinction.
  label?: string;
  // Mute the label entirely if the caller wants a more subtle UI
  // (e.g. as a tail cursor inside an already-streaming bubble).
  silent?: boolean;
};

export function StreamingDots({ label = "Thinking", silent = false }: Props) {
  return (
    <View className="flex-row items-center gap-2 py-1">
      <View className="flex-row items-center gap-1">
        <Dot delay={0} />
        <Dot delay={140} />
        <Dot delay={280} />
      </View>
      {!silent ? (
        <Text variant="muted" className="text-xs">
          {label}
        </Text>
      ) : null}
    </View>
  );
}

function Dot({ delay }: { delay: number }) {
  const opacity = useRef(new Animated.Value(0.25)).current;
  useEffect(() => {
    // Two-step fade gives a cleaner "pulse" than a single sine — the
    // dot lights up then drops back to the baseline for the next
    // sibling.
    const loop = Animated.loop(
      Animated.sequence([
        Animated.delay(delay),
        Animated.timing(opacity, {
          toValue: 1,
          duration: 360,
          easing: Easing.out(Easing.quad),
          useNativeDriver: USE_NATIVE_DRIVER
        }),
        Animated.timing(opacity, {
          toValue: 0.25,
          duration: 360,
          easing: Easing.in(Easing.quad),
          useNativeDriver: USE_NATIVE_DRIVER
        })
      ])
    );
    loop.start();
    return () => loop.stop();
  }, [delay, opacity]);
  return (
    <Animated.View
      style={{
        width: 5,
        height: 5,
        borderRadius: 999,
        backgroundColor: "#9AA4B2",
        opacity
      }}
    />
  );
}
