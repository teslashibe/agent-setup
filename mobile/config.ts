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
