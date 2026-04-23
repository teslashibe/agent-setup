import type { ComponentType } from "react";
import { Pressable, View } from "react-native";

import { Text } from "@/components/ui/Text";
import { brand, type BrandSuggestion } from "@/branding";

// ChatEmptyState is what the operator sees on a brand-new chat
// before they've sent their first message. It does three jobs:
//
//  1. Establishes who the agent is (brand persona) so the operator
//     doesn't have to guess from "the title is 'New chat'".
//  2. Sets capability expectations via the suggestion list.
//  3. Gives a few starter prompts that map to the most common
//     workflows. Tapping a chip writes that prompt into the
//     composer so the operator can refine before sending.
//
// The agent name, description, hero glyph, and suggestion list all
// come from `branding.ts`. Forks rebrand by editing that surface —
// this component stays generic.

type Props = {
  // Override the brand-default agent name for this empty state.
  agentName?: string;
  // Override the brand-default description for this empty state.
  description?: string;
  // Override the brand-default hero glyph for this empty state.
  HeroIcon?: ComponentType<{ size: number; color: string }>;
  // Override the brand-default suggestion list. Pass an empty array
  // to hide the suggestions section entirely.
  suggestions?: BrandSuggestion[];
  // Called with the chip's `prompt` text when the operator taps a
  // suggestion — the parent typically writes it into the composer.
  onPick: (prompt: string) => void;
};

export function ChatEmptyState({
  agentName,
  description,
  HeroIcon,
  suggestions,
  onPick
}: Props) {
  const Icon = HeroIcon ?? brand.AvatarIcon;
  const name = agentName ?? brand.name;
  const desc = description ?? brand.description;
  const items = suggestions ?? brand.emptyStateSuggestions;

  return (
    <View className="flex-1 items-center justify-center px-5 py-10">
      <View className="w-full max-w-md gap-6">
        <View className="items-center gap-3">
          <View
            className="h-14 w-14 items-center justify-center rounded-2xl"
            style={{ backgroundColor: "rgba(0, 212, 170, 0.15)" }}
          >
            <Icon size={28} color="#00D4AA" />
          </View>
          <Text variant="h3" className="text-center">
            {name}
          </Text>
          <Text variant="muted" className="text-center text-sm">
            {desc}
          </Text>
        </View>

        {items.length > 0 ? (
          <View className="gap-2">
            <Text
              variant="muted"
              className="text-center text-[10px] uppercase tracking-wider"
            >
              Try one of these
            </Text>
            <View className="gap-2">
              {items.map((s) => (
                <SuggestionChip key={s.id} suggestion={s} onPick={onPick} />
              ))}
            </View>
          </View>
        ) : null}
      </View>
    </View>
  );
}

function SuggestionChip({
  suggestion,
  onPick
}: {
  suggestion: BrandSuggestion;
  onPick: (prompt: string) => void;
}) {
  const Icon = suggestion.icon;
  return (
    <Pressable
      onPress={() => onPick(suggestion.prompt)}
      className="flex-row items-center gap-3 rounded-xl border border-border bg-card px-3 py-2.5 active:bg-secondary"
      accessibilityLabel={suggestion.title}
    >
      <View
        className="h-8 w-8 items-center justify-center rounded-lg"
        style={{ backgroundColor: "rgba(0, 212, 170, 0.1)" }}
      >
        <Icon size={16} color="#00D4AA" />
      </View>
      <View className="flex-1 gap-0.5">
        <Text variant="small" className="font-semibold text-foreground">
          {suggestion.title}
        </Text>
        <Text variant="muted" className="text-xs" numberOfLines={2}>
          {suggestion.prompt}
        </Text>
      </View>
    </Pressable>
  );
}
