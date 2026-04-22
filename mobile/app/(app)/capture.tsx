import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Pressable, ScrollView, Switch, View } from "react-native";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Text } from "@/components/ui/Text";
import { useNotificationCapture } from "@/providers/NotificationCaptureProvider";
import { listCapturedApps, type AppSummary } from "@/services/notifications";

/**
 * Allowlist seed populated with the contact-heavy apps the pilot user
 * actually relies on (real estate workflow). The list is intentionally
 * editable below so other forks can tune it for their persona.
 */
const SUGGESTED_APPS: { pkg: string; label: string }[] = [
  { pkg: "com.google.android.apps.messaging", label: "Messages (Google)" },
  { pkg: "com.android.mms", label: "Messages (AOSP)" },
  { pkg: "com.whatsapp", label: "WhatsApp" },
  { pkg: "com.google.android.gm", label: "Gmail" },
  { pkg: "com.microsoft.office.outlook", label: "Outlook" },
  { pkg: "com.zillow.android.zillowmap", label: "Zillow" },
  { pkg: "com.zillow.android.rentals", label: "Zillow Rentals" },
  { pkg: "com.realtor.android", label: "Realtor.com" },
  { pkg: "com.facebook.orca", label: "Messenger" },
  { pkg: "com.android.dialer", label: "Phone (AOSP)" },
  { pkg: "com.google.android.dialer", label: "Phone (Google)" },
];

function formatLastSync(d: Date | null): string {
  if (!d) return "never";
  const diffMs = Date.now() - d.getTime();
  const minutes = Math.round(diffMs / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return d.toLocaleString();
}

export default function NotificationCaptureScreen() {
  const capture = useNotificationCapture();
  const [customPkg, setCustomPkg] = useState("");
  const [apps, setApps] = useState<AppSummary[]>([]);
  const [appsError, setAppsError] = useState<string | null>(null);
  const [loadingApps, setLoadingApps] = useState(false);
  const [syncing, setSyncing] = useState(false);

  const allowlist = capture.allowlist;
  const allowlistSet = useMemo(() => new Set(allowlist), [allowlist]);

  const refreshApps = useCallback(async () => {
    if (!capture.isAvailable) return;
    setLoadingApps(true);
    setAppsError(null);
    try {
      const next = await listCapturedApps();
      setApps(next);
    } catch (err) {
      setAppsError(err instanceof Error ? err.message : "Failed to load apps");
    } finally {
      setLoadingApps(false);
    }
  }, [capture.isAvailable]);

  useEffect(() => {
    void refreshApps();
  }, [refreshApps]);

  const toggleApp = useCallback(
    (pkg: string) => {
      const next = allowlistSet.has(pkg)
        ? allowlist.filter((p) => p !== pkg)
        : [...allowlist, pkg];
      capture.setAllowlist(next);
    },
    [allowlist, allowlistSet, capture],
  );

  const addCustomApp = useCallback(() => {
    const pkg = customPkg.trim();
    if (!pkg) return;
    if (allowlistSet.has(pkg)) {
      setCustomPkg("");
      return;
    }
    capture.setAllowlist([...allowlist, pkg]);
    setCustomPkg("");
  }, [allowlist, allowlistSet, capture, customPkg]);

  const handleEnable = useCallback(
    (next: boolean) => {
      if (next && !capture.hasPermission) {
        Alert.alert(
          "Permission required",
          "Capture needs Notification Access. We'll open the system settings page so you can grant it for Agent App.",
          [
            { text: "Cancel", style: "cancel" },
            {
              text: "Open settings",
              onPress: () => capture.openPermissionSettings(),
            },
          ],
        );
        return;
      }
      capture.setEnabled(next);
    },
    [capture],
  );

  const handleFlush = useCallback(async () => {
    setSyncing(true);
    try {
      const accepted = await capture.flushNow();
      Alert.alert("Sync complete", `${accepted} new notification${accepted === 1 ? "" : "s"} uploaded.`);
      await refreshApps();
    } catch (err) {
      Alert.alert("Sync failed", err instanceof Error ? err.message : "Unknown error");
    } finally {
      setSyncing(false);
    }
  }, [capture, refreshApps]);

  if (!capture.isAvailable) {
    return (
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 20 }}>
        <Card>
          <CardHeader>
            <CardTitle>Notification Capture</CardTitle>
            <CardDescription>Not available for this build.</CardDescription>
          </CardHeader>
          <CardContent>
            <Text variant="small" className="text-muted">
              The notification capture pipeline is Android-only and must be enabled in the
              deployment via NOTIFICATIONS_ENABLED. iOS does not expose a comparable system API
              without an app extension.
            </Text>
          </CardContent>
        </Card>
      </ScrollView>
    );
  }

  return (
    <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 20, paddingBottom: 120 }}>
      <View className="gap-4">
        <Card>
          <CardHeader>
            <CardTitle>Capture status</CardTitle>
            <CardDescription>
              When enabled, the agent can produce a daily rollup across every monitored app.
            </CardDescription>
          </CardHeader>
          <CardContent className="gap-3">
            <View className="flex-row items-center justify-between">
              <View className="flex-1 pr-3">
                <Text variant="p">Enable capture</Text>
                <Text variant="small" className="text-muted">
                  Master switch. Disable to pause without revoking system permission.
                </Text>
              </View>
              <Switch value={capture.isEnabled} onValueChange={handleEnable} />
            </View>
            <View className="flex-row items-center justify-between">
              <View className="flex-1 pr-3">
                <Text variant="p">Notification access</Text>
                <Text variant="small" className="text-muted">
                  {capture.hasPermission ? "Granted" : "Not granted — open settings to allow"}
                </Text>
              </View>
              <Button size="sm" variant="outline" onPress={() => capture.openPermissionSettings()}>
                {capture.hasPermission ? "Open settings" : "Grant access"}
              </Button>
            </View>
            <View className="rounded-lg border border-border p-3">
              <Text variant="small" className="text-muted">
                Pending in local buffer
              </Text>
              <Text variant="large">{capture.pendingCount}</Text>
              <Text variant="small" className="text-muted mt-1">
                Last sync: {formatLastSync(capture.lastSyncAt)}
              </Text>
            </View>
            <Button onPress={handleFlush} loading={syncing}>
              Sync now
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Monitored apps</CardTitle>
            <CardDescription>
              Only notifications from these apps are captured and sent to the agent.
            </CardDescription>
          </CardHeader>
          <CardContent className="gap-3">
            {SUGGESTED_APPS.map((app) => {
              const on = allowlistSet.has(app.pkg);
              return (
                <Pressable
                  key={app.pkg}
                  onPress={() => toggleApp(app.pkg)}
                  className="flex-row items-center justify-between rounded-lg border border-border p-3 active:opacity-80"
                >
                  <View className="flex-1 pr-3">
                    <Text variant="p">{app.label}</Text>
                    <Text variant="small" className="text-muted">
                      {app.pkg}
                    </Text>
                  </View>
                  <Switch value={on} onValueChange={() => toggleApp(app.pkg)} />
                </Pressable>
              );
            })}

            {allowlist
              .filter((pkg) => !SUGGESTED_APPS.some((a) => a.pkg === pkg))
              .map((pkg) => (
                <Pressable
                  key={pkg}
                  onPress={() => toggleApp(pkg)}
                  className="flex-row items-center justify-between rounded-lg border border-border p-3 active:opacity-80"
                >
                  <View className="flex-1 pr-3">
                    <Text variant="p">{pkg}</Text>
                    <Text variant="small" className="text-muted">
                      Custom
                    </Text>
                  </View>
                  <Switch value={true} onValueChange={() => toggleApp(pkg)} />
                </Pressable>
              ))}

            <View className="gap-2">
              <Input
                label="Add custom package"
                placeholder="com.example.app"
                value={customPkg}
                onChangeText={setCustomPkg}
                autoCapitalize="none"
                autoCorrect={false}
              />
              <Button size="sm" variant="outline" onPress={addCustomApp}>
                Add to allowlist
              </Button>
            </View>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Captured apps</CardTitle>
            <CardDescription>What the backend has actually received recently.</CardDescription>
          </CardHeader>
          <CardContent className="gap-2">
            {loadingApps ? (
              <Text variant="small" className="text-muted">
                Loading…
              </Text>
            ) : appsError ? (
              <Text variant="small" className="text-destructive">
                {appsError}
              </Text>
            ) : apps.length === 0 ? (
              <Text variant="small" className="text-muted">
                Nothing captured yet. Enable capture, grant notification access, and trigger a
                test notification.
              </Text>
            ) : (
              apps.map((app) => (
                <View key={app.app_package} className="flex-row justify-between">
                  <View className="flex-1 pr-3">
                    <Text variant="p">{app.app_label || app.app_package}</Text>
                    <Text variant="small" className="text-muted">
                      Last: {new Date(app.last_at).toLocaleString()}
                    </Text>
                  </View>
                  <Text variant="p">{app.count}</Text>
                </View>
              ))
            )}
          </CardContent>
        </Card>
      </View>
    </ScrollView>
  );
}
