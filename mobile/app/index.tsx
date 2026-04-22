import { ActivityIndicator, View } from "react-native";
import { Redirect } from "expo-router";

import { useAuthSession } from "@/providers/AuthSessionProvider";

// Dev-only escape hatch: when EXPO_PUBLIC_DEV_BYPASS_AUTH=true the root
// route always lands inside the (app) shell so we can preview UI without
// going through magic-link auth. Off by default; never enabled in prod.
const BYPASS_AUTH = process.env.EXPO_PUBLIC_DEV_BYPASS_AUTH === "true";

export default function IndexScreen() {
  const { isLoading, isAuthenticated } = useAuthSession();

  if (isLoading) {
    return (
      <View className="flex-1 items-center justify-center bg-background">
        <ActivityIndicator color="#00D4AA" />
      </View>
    );
  }

  const target = BYPASS_AUTH || isAuthenticated ? "/(app)" : "/(auth)/welcome";
  return <Redirect href={target} />;
}
