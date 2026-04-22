import { Redirect, Tabs } from "expo-router";
import { Platform } from "react-native";

import { FloatingTabBar } from "@/components/FloatingTabBar";
import { useAuthSession } from "@/providers/AuthSessionProvider";
import { TEAMS_ENABLED } from "@/config";

// Dev-only escape hatch so the web preview can render the (app) shell
// without going through magic-link auth. Gated behind an env var so it
// is impossible to accidentally enable in a production build.
const BYPASS_AUTH = process.env.EXPO_PUBLIC_DEV_BYPASS_AUTH === "true";

export default function AppLayout() {
  const { isLoading, isAuthenticated } = useAuthSession();

  if (!BYPASS_AUTH && !isLoading && !isAuthenticated) {
    return <Redirect href="/(auth)/welcome" />;
  }

  // Layout shape differs per platform:
  //   - Web: the custom FloatingTabBar paints itself as a fixed 240px
  //     sidebar on the left (CSS position: fixed). We hide the default
  //     tab-bar layout and push the scene content right via sceneStyle
  //     marginLeft so screens flow into the remaining viewport.
  //   - Native: the custom FloatingTabBar is an absolutely positioned
  //     floating bottom bar overlaid on content. Hiding the default tab
  //     bar layout stops bottom-tabs from reserving space for it.
  const screenOptions = Platform.select({
    web: {
      headerShown: false,
      tabBarStyle: { display: "none" as const },
      sceneStyle: { marginLeft: 240 }
    },
    default: {
      headerShown: false,
      tabBarStyle: { display: "none" as const }
    }
  });

  return (
    <Tabs tabBar={(props) => <FloatingTabBar {...props} />} screenOptions={screenOptions}>
      <Tabs.Screen name="index" options={{ title: "Chats" }} />
      {/* When TEAMS_ENABLED is false the routes still exist (so deep links
          don't 404), but href: null hides the Teams pill from the tab bar. */}
      <Tabs.Screen
        name="teams"
        options={TEAMS_ENABLED ? { title: "Teams" } : { title: "Teams", href: null }}
      />
      <Tabs.Screen name="settings" options={{ title: "Settings" }} />
      <Tabs.Screen name="chat" options={{ href: null }} />
      <Tabs.Screen name="platforms" options={{ href: null }} />
      {/* Capture screen is reached via Settings → Notification Capture; we
          register it as a hidden tab so deep links work and expo-router
          knows about the route. */}
      <Tabs.Screen name="capture" options={{ href: null }} />
    </Tabs>
  );
}
