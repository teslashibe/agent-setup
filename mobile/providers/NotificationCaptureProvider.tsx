import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { AppState, type AppStateStatus } from "react-native";

import {
  bufferSize,
  getAllowlist,
  hasPermission,
  isCaptureAvailable,
  isEnabled as nativeIsEnabled,
  lastSyncAt as nativeLastSyncAt,
  openSettings as nativeOpenSettings,
  setAllowlist as nativeSetAllowlist,
  setEnabled as nativeSetEnabled,
} from "../modules/notification-capture/src";

import { useAuthSession } from "@/providers/AuthSessionProvider";
import { flushNow, startSync, stopSync } from "@/services/notificationSync";
import { NOTIFICATIONS_CAPTURE_ENABLED } from "@/config";

type NotificationCaptureContextValue = {
  isAvailable: boolean;
  isEnabled: boolean;
  hasPermission: boolean;
  allowlist: string[];
  pendingCount: number;
  lastSyncAt: Date | null;
  setEnabled: (enabled: boolean) => void;
  setAllowlist: (packages: string[]) => void;
  openPermissionSettings: () => void;
  flushNow: () => Promise<number>;
  refresh: () => void;
};

const NotificationCaptureContext = createContext<NotificationCaptureContextValue | null>(
  null,
);

const FALLBACK_VALUE: NotificationCaptureContextValue = {
  isAvailable: false,
  isEnabled: false,
  hasPermission: false,
  allowlist: [],
  pendingCount: 0,
  lastSyncAt: null,
  setEnabled: () => undefined,
  setAllowlist: () => undefined,
  openPermissionSettings: () => undefined,
  flushNow: async () => 0,
  refresh: () => undefined,
};

/**
 * NotificationCaptureProvider lifts the native module's session-shaped
 * state into React. It runs only when:
 *   1. The deployment opted into the feature (EXPO_PUBLIC_NOTIFICATIONS_ENABLED=true).
 *   2. The native module is actually present (Android + a build that
 *      includes modules/notification-capture).
 *   3. The user is authenticated (uploads need a JWT).
 *
 * When any of those checks fail the provider returns a frozen
 * "unavailable" context so consumers can still render conditional UI
 * without a separate availability hook.
 *
 * The provider is purely a React surface: persistence (allowlist, master
 * switch) lives on the native side; transport (uploadBatch) lives in
 * services/notificationSync.ts. The provider only orchestrates start/stop
 * of the foreground flush loop based on the master switch + auth state.
 */
export function NotificationCaptureProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated } = useAuthSession();
  const [isEnabled, setIsEnabled] = useState(false);
  const [permission, setPermission] = useState(false);
  const [allowlist, setAllowlistState] = useState<string[]>([]);
  const [pending, setPending] = useState(0);
  const [lastSync, setLastSync] = useState<Date | null>(null);

  const featureOn = NOTIFICATIONS_CAPTURE_ENABLED && isCaptureAvailable;

  const refresh = useCallback(() => {
    if (!featureOn) {
      setIsEnabled(false);
      setPermission(false);
      setAllowlistState([]);
      setPending(0);
      setLastSync(null);
      return;
    }
    setIsEnabled(nativeIsEnabled());
    setPermission(hasPermission());
    setAllowlistState(getAllowlist());
    setPending(bufferSize());
    setLastSync(nativeLastSyncAt());
  }, [featureOn]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (!featureOn) return;
    const sub = AppState.addEventListener("change", (state: AppStateStatus) => {
      if (state === "active") {
        refresh();
      }
    });
    return () => sub.remove();
  }, [featureOn, refresh]);

  useEffect(() => {
    if (!featureOn || !isAuthenticated) {
      stopSync();
      return;
    }
    if (isEnabled) {
      startSync();
      void flushNow().then((accepted) => {
        if (accepted > 0) {
          refresh();
        }
      });
    } else {
      stopSync();
    }
    return () => stopSync();
  }, [featureOn, isAuthenticated, isEnabled, refresh]);

  const setEnabled = useCallback(
    (enabled: boolean) => {
      if (!featureOn) return;
      nativeSetEnabled(enabled);
      setIsEnabled(enabled);
    },
    [featureOn],
  );

  const setAllowlist = useCallback(
    (packages: string[]) => {
      if (!featureOn) return;
      nativeSetAllowlist(packages);
      setAllowlistState(packages);
    },
    [featureOn],
  );

  const openPermissionSettings = useCallback(() => {
    if (!featureOn) return;
    nativeOpenSettings();
  }, [featureOn]);

  const flush = useCallback(async () => {
    const accepted = await flushNow();
    refresh();
    return accepted;
  }, [refresh]);

  const value = useMemo<NotificationCaptureContextValue>(
    () =>
      featureOn
        ? {
            isAvailable: true,
            isEnabled,
            hasPermission: permission,
            allowlist,
            pendingCount: pending,
            lastSyncAt: lastSync,
            setEnabled,
            setAllowlist,
            openPermissionSettings,
            flushNow: flush,
            refresh,
          }
        : FALLBACK_VALUE,
    [
      featureOn,
      isEnabled,
      permission,
      allowlist,
      pending,
      lastSync,
      setEnabled,
      setAllowlist,
      openPermissionSettings,
      flush,
      refresh,
    ],
  );

  return (
    <NotificationCaptureContext.Provider value={value}>
      {children}
    </NotificationCaptureContext.Provider>
  );
}

export function useNotificationCapture(): NotificationCaptureContextValue {
  const ctx = useContext(NotificationCaptureContext);
  return ctx ?? FALLBACK_VALUE;
}
