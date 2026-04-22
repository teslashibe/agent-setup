import { useEffect } from "react";
import { ActivityIndicator, View } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";

// Spec alias for the invite landing — the public deep-link landing lives at
// /invites/accept?token=… so it's reachable both before and after auth, but
// the audit (L3) calls for an /(auth)/invite path so signed-out users have a
// natural entry point inside the auth navigator. We forward to the canonical
// route preserving the token so all behaviour stays in one screen.
export default function InviteAuthAlias() {
  const router = useRouter();
  const params = useLocalSearchParams<{ token?: string }>();
  const token = typeof params.token === "string" ? params.token : "";

  useEffect(() => {
    router.replace({
      pathname: "/invites/accept",
      params: token ? { token } : {},
    });
  }, [router, token]);

  return (
    <View className="flex-1 items-center justify-center bg-background">
      <ActivityIndicator color="#00D4AA" />
    </View>
  );
}
