import { Platform } from "react-native";

export const API_URL =
  process.env.EXPO_PUBLIC_API_URL ??
  Platform.select({
    ios: "http://localhost:8080",
    android: "http://10.0.2.2:8080",
    default: "http://localhost:8080"
  })!;

// TEAMS_ENABLED mirrors the server-side TEAMS_ENABLED flag so the UI can
// hide the Teams tab + settings team-switcher when the operator turns the
// feature off. Defaults to enabled to match the server default; setting
// EXPO_PUBLIC_TEAMS_ENABLED="false" (string) opts the build out.
//
// Spec: .cursor/tickets/teams-scope.md §"Configuration":
//   `TEAMS_ENABLED = process.env.EXPO_PUBLIC_TEAMS_ENABLED !== "false"`
export const TEAMS_ENABLED = process.env.EXPO_PUBLIC_TEAMS_ENABLED !== "false";

// NOTIFICATIONS_CAPTURE_ENABLED mirrors the server-side NOTIFICATIONS_ENABLED
// flag. Default OFF — the feature is opt-in per the notification-capture
// scope so forks of the template that don't ship the Android capture
// pipeline pay zero UI overhead.
//
// When false, the capture settings screen, provider, and any tab entries
// remain inert. Set EXPO_PUBLIC_NOTIFICATIONS_ENABLED="true" to opt in.
export const NOTIFICATIONS_CAPTURE_ENABLED =
  process.env.EXPO_PUBLIC_NOTIFICATIONS_ENABLED === "true";
