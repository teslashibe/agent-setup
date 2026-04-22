import { Platform } from "react-native";
import { requireOptionalNativeModule } from "expo";

/**
 * Type contract exposed by the native NotificationCaptureModule. Mirrors
 * the Kotlin functions one-to-one. All methods are synchronous (the
 * underlying SharedPreferences ops are O(1) at the volumes we care about).
 *
 * On non-Android platforms `nativeModule` is null and the helper functions
 * below short-circuit to safe defaults so calling code can stay
 * platform-agnostic.
 */
export interface NotificationCaptureNativeModule {
  hasPermission(): boolean;
  openSettings(): void;
  isEnabled(): boolean;
  setEnabled(enabled: boolean): void;
  getAllowlist(): string[];
  setAllowlist(packages: string[]): void;
  drainBuffer(): string;
  requeueEvents(eventsJson: string): void;
  bufferSize(): number;
  lastSyncAt(): number;
  markSynced(timestampMs: number): void;
}

/**
 * The shape of one notification event captured by the native side. Field
 * names must match `notifications.EventInput` on the backend so the JS
 * layer can forward the buffer verbatim.
 */
export interface CapturedNotification {
  app_package: string;
  app_label: string;
  title: string;
  content: string;
  category: string;
  captured_at: string;
}

const nativeModule = requireOptionalNativeModule<NotificationCaptureNativeModule>(
  "NotificationCapture"
);

export const isCaptureAvailable = Platform.OS === "android" && nativeModule != null;

function unavailable<T>(fallback: T): T {
  if (__DEV__ && Platform.OS === "android" && !nativeModule) {
    console.warn(
      "[notification-capture] Native module is missing — was the project rebuilt after adding the module?"
    );
  }
  return fallback;
}

export function hasPermission(): boolean {
  return nativeModule ? nativeModule.hasPermission() : unavailable(false);
}

export function openSettings(): void {
  if (nativeModule) {
    nativeModule.openSettings();
  }
}

export function isEnabled(): boolean {
  return nativeModule ? nativeModule.isEnabled() : unavailable(false);
}

export function setEnabled(enabled: boolean): void {
  if (nativeModule) {
    nativeModule.setEnabled(enabled);
  }
}

export function getAllowlist(): string[] {
  return nativeModule ? nativeModule.getAllowlist() : unavailable([]);
}

export function setAllowlist(packages: string[]): void {
  if (nativeModule) {
    nativeModule.setAllowlist(packages);
  }
}

/**
 * Drain the on-disk buffer atomically. Returns an array of captured
 * events ready to upload. The native side clears the buffer in the same
 * call so callers must persist (or upload) the result; data is lost if
 * the result is dropped.
 */
export function drainBuffer(): CapturedNotification[] {
  if (!nativeModule) {
    return [];
  }
  const raw = nativeModule.drainBuffer();
  try {
    const parsed = JSON.parse(raw) as CapturedNotification[];
    return Array.isArray(parsed) ? parsed : [];
  } catch (err) {
    if (__DEV__) {
      console.warn("[notification-capture] drainBuffer parse error:", err);
    }
    return [];
  }
}

/**
 * Prepend events back to the native buffer. Call this on upload failure
 * with the events that were in the chunk that failed. The native side
 * preserves freshest-first ordering and caps at the buffer max — older
 * entries are dropped if the requeue would overflow.
 */
export function requeueEvents(events: CapturedNotification[]): void {
  if (!nativeModule || events.length === 0) return;
  nativeModule.requeueEvents(JSON.stringify(events));
}

export function bufferSize(): number {
  return nativeModule ? nativeModule.bufferSize() : 0;
}

export function lastSyncAt(): Date | null {
  if (!nativeModule) return null;
  const ms = nativeModule.lastSyncAt();
  return ms > 0 ? new Date(ms) : null;
}

export function markSynced(when: Date = new Date()): void {
  if (nativeModule) {
    nativeModule.markSynced(when.getTime());
  }
}
