import { Stack } from "expo-router";

// Stack inside the Teams tab so navigating list → new → [id]/index pushes
// rather than swapping tabs. headerShown: false because each child draws its
// own header with consistent padding + a back button.
export default function TeamsLayout() {
  return <Stack screenOptions={{ headerShown: false }} />;
}
