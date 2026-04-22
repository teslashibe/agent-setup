import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, KeyboardAvoidingView, Linking, Platform as RNPlatform, ScrollView, View } from "react-native";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Separator } from "@/components/ui/Separator";
import { Text } from "@/components/ui/Text";
import {
  buildCredential,
  COOKIE_EDITOR_LINK,
  PLATFORMS,
  PlatformMetadata,
  PlatformStatus,
  disconnectPlatform,
  listPlatforms,
  setPlatformCredential
} from "@/services/platforms";

type ConnectFormProps = {
  meta: PlatformMetadata;
  onSubmit: (values: Record<string, string>, label: string) => Promise<void>;
  busy: boolean;
};

function ConnectForm({ meta, onSubmit, busy }: ConnectFormProps) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [label, setLabel] = useState("");
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    setError(null);
    const required = meta.fields.filter((f) => !f.label.toLowerCase().includes("optional"));
    for (const f of required) {
      if (!values[f.name]?.trim()) {
        setError(`${f.label} is required`);
        return;
      }
    }
    try {
      await onSubmit(values, label);
      setValues({});
      setLabel("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save credentials");
    }
  };

  if (meta.noCredentials) {
    return (
      <Text variant="small" className="text-muted">
        {meta.helper}
      </Text>
    );
  }

  return (
    <View className="gap-3">
      <Text variant="small" className="text-muted">
        {meta.helper}
      </Text>
      <Button variant="ghost" size="sm" onPress={() => Linking.openURL(COOKIE_EDITOR_LINK)}>
        Get Cookie-Editor extension
      </Button>
      {meta.fields.map((f) => (
        <Input
          key={f.name}
          label={f.label}
          value={values[f.name] ?? ""}
          onChangeText={(text) => setValues((prev) => ({ ...prev, [f.name]: text }))}
          autoCapitalize="none"
          autoCorrect={false}
          secureTextEntry={f.kind === "token"}
          placeholder={f.placeholder ?? "Paste value"}
          multiline={f.kind === "cookie" && f.name === "cookies"}
          editable={!busy}
        />
      ))}
      <Input
        label="Label (optional)"
        value={label}
        onChangeText={setLabel}
        autoCapitalize="none"
        placeholder="Personal account, work, etc."
        editable={!busy}
      />
      {error ? (
        <Text variant="small" className="text-destructive">
          {error}
        </Text>
      ) : null}
      <Button onPress={handleSubmit} disabled={busy}>
        {busy ? "Saving…" : "Save credentials"}
      </Button>
    </View>
  );
}

type RowProps = {
  meta: PlatformMetadata;
  status?: PlatformStatus;
  onChanged: () => void | Promise<void>;
};

function PlatformRow({ meta, status, onChanged }: RowProps) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const isConnected = !!status?.connected;

  const handleSubmit = async (values: Record<string, string>, label: string) => {
    setBusy(true);
    try {
      const credential = buildCredential(meta.fields, values);
      await setPlatformCredential(meta.id, credential, label);
      await onChanged();
      setOpen(false);
    } finally {
      setBusy(false);
    }
  };

  const handleDisconnect = () => {
    Alert.alert("Disconnect platform", `Remove stored credentials for ${meta.name}?`, [
      { text: "Cancel", style: "cancel" },
      {
        text: "Disconnect",
        style: "destructive",
        onPress: async () => {
          setBusy(true);
          try {
            await disconnectPlatform(meta.id);
            await onChanged();
          } catch (e) {
            Alert.alert("Could not disconnect", e instanceof Error ? e.message : "Unknown error");
          } finally {
            setBusy(false);
          }
        }
      }
    ]);
  };

  return (
    <Card>
      <CardHeader>
        <View className="flex-row items-center justify-between">
          <CardTitle>{meta.name}</CardTitle>
          {meta.noCredentials ? (
            <Badge variant="outline">No auth</Badge>
          ) : isConnected ? (
            <Badge variant="default">Connected</Badge>
          ) : (
            <Badge variant="secondary">Not connected</Badge>
          )}
        </View>
        {status?.summary?.label ? <CardDescription>{status.summary.label}</CardDescription> : null}
      </CardHeader>
      <CardContent>
        {meta.noCredentials ? (
          <Text variant="small" className="text-muted">
            {meta.helper}
          </Text>
        ) : (
          <View className="gap-3">
            <View className="flex-row gap-2">
              <Button
                variant={isConnected ? "secondary" : "default"}
                size="sm"
                onPress={() => setOpen((v) => !v)}
                disabled={busy}
              >
                {open ? "Cancel" : isConnected ? "Replace credentials" : "Connect"}
              </Button>
              {isConnected ? (
                <Button variant="destructive" size="sm" onPress={handleDisconnect} disabled={busy}>
                  Disconnect
                </Button>
              ) : null}
            </View>
            {open ? <ConnectForm meta={meta} onSubmit={handleSubmit} busy={busy} /> : null}
          </View>
        )}
      </CardContent>
    </Card>
  );
}

export default function PlatformsScreen() {
  const [statuses, setStatuses] = useState<PlatformStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const list = await listPlatforms();
      setStatuses(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not load platforms");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const byPlatform = useMemo(() => {
    const map = new Map<string, PlatformStatus>();
    for (const s of statuses) map.set(s.platform, s);
    return map;
  }, [statuses]);

  return (
    <KeyboardAvoidingView
      behavior={RNPlatform.OS === "ios" ? "padding" : undefined}
      className="flex-1 bg-background"
    >
      <ScrollView contentContainerStyle={{ padding: 20, paddingBottom: 120 }}>
        <View className="gap-4">
          <View className="gap-2">
            <Text variant="h2">Platform Connections</Text>
            <Text variant="small" className="text-muted">
              Paste authentication cookies or API tokens for each platform you want the agent to use. Credentials are
              encrypted at rest and only ever sent to the matching scraper.
            </Text>
          </View>
          <Separator />
          {loading ? (
            <Text>Loading…</Text>
          ) : error ? (
            <EmptyState
              title="Couldn't load platforms"
              description={error}
              actionLabel="Retry"
              onAction={refresh}
            />
          ) : (
            PLATFORMS.map((meta) => (
              <PlatformRow key={meta.id} meta={meta} status={byPlatform.get(meta.id)} onChanged={refresh} />
            ))
          )}
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
