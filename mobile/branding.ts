import type { ComponentType } from "react";
import { Sparkles } from "lucide-react-native";

// branding.ts is the single override surface a fork uses to give the
// chat UI a brand identity without copy-pasting the chat components.
//
// The template ships sensible "Agent Setup" defaults so an unforked
// build still looks intentional out of the box. To rebrand:
//
//  1. Fork the repo.
//  2. Replace this file with your own export — same shape, your
//     copy + glyph + suggestion list.
//  3. Optionally re-export `brand` from a lazier place (e.g. a
//     server-driven config) once you outgrow the static surface.
//
// Anything that ISN'T in this surface stays generic on purpose. The
// accent color is owned by `theme/` because Tailwind tokens are
// resolved at build time; the agent-loop wiring lives in the backend
// `brand/` package; everything else flows from these defaults.

// AgentBrand is the shape consumers pull from `brand` below.
//
// AgentName / AgentSubtitle are short strings. They get rendered into
// the chat header (left of the live indicator) and the empty-state
// hero (centered above the prompt suggestions).
//
// AvatarIcon is a lucide icon component (or anything that accepts the
// same `{ size, color }` props). We avoid bundling an Image asset
// here because forks can pick the same teal accent from `theme/` and
// keep the avatar a single React tree node.
//
// EmptyStateSuggestions is the curated starter prompt list shown on a
// brand-new chat. Keep it short — 3-5 items. Each prompt is the
// literal text that lands in the composer when the chip is tapped, so
// write them in the operator's voice, not the agent's.
export type AgentBrand = {
  name: string;
  subtitle: string;
  description: string;
  AvatarIcon: ComponentType<{ size: number; color: string }>;
  emptyStateSuggestions: BrandSuggestion[];
};

export type BrandSuggestion = {
  id: string;
  icon: ComponentType<{ size: number; color: string }>;
  title: string;
  prompt: string;
};

// brand is the active brand for this build. Forks override the whole
// object. The default below intentionally matches the template's
// "Agent Setup" identity so the chat surface is presentable for a
// developer just kicking the tires.
export const brand: AgentBrand = {
  name: "Agent",
  subtitle: "Posting agent",
  description:
    "Drafts brand-aligned posts, reads channel rules, and only publishes after your approval.",
  AvatarIcon: Sparkles,
  emptyStateSuggestions: [
    {
      id: "intro",
      icon: Sparkles,
      title: "What can you do?",
      prompt:
        "Walk me through what tools you have access to and what you're built to help with."
    },
    {
      id: "draft",
      icon: Sparkles,
      title: "Draft a post",
      prompt:
        "Draft a short post for the channel I usually publish to. Read the channel's rules first, then show me the draft — don't post yet."
    }
  ]
};
