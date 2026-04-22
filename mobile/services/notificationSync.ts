import { AppState, type AppStateStatus, type NativeEventSubscription } from "react-native";

import {
  bufferSize,
  drainBuffer,
  isCaptureAvailable,
  isEnabled,
  markSynced,
  requeueEvents,
} from "../modules/notification-capture/src";

import { uploadBatch } from "@/services/notifications";

/**
 * Cadence at which the foreground sync loop runs. Five minutes balances
 * "fresh enough that the rollup feels current" against "don't drain the
 * battery" — the sync work is tiny but every wake costs.
 */
const FLUSH_INTERVAL_MS = 5 * 60 * 1000;

/**
 * Hard cap on events shipped per HTTP request. Above this we split into
 * sequential POSTs. Matches NOTIFICATIONS_MAX_PAGE_SIZE on the server so
 * we never get rejected for over-sized batches.
 */
const MAX_EVENTS_PER_REQUEST = 200;

let intervalHandle: ReturnType<typeof setInterval> | null = null;
let appStateSubscription: NativeEventSubscription | null = null;
let inflight = false;

/**
 * Singleton flush — guards against re-entrance from overlapping interval
 * + appforegrounded events. Returns the number of accepted events for
 * the caller to surface in the UI ("X notifications synced").
 *
 * Failure handling: drainBuffer() empties the native store atomically.
 * If a chunk upload throws (network drop, auth expiry, server 500) we
 * requeue every event that hasn't been accepted yet so the next flush
 * picks them up. This is the property a real-estate agent driving
 * between properties on flaky cell needs — no notification is ever
 * silently lost because of a bad signal.
 */
export async function flushNow(): Promise<number> {
  if (!isCaptureAvailable || !isEnabled() || inflight) {
    return 0;
  }
  inflight = true;
  try {
    const events = drainBuffer();
    if (events.length === 0) {
      return 0;
    }
    let accepted = 0;
    for (let i = 0; i < events.length; i += MAX_EVENTS_PER_REQUEST) {
      const chunk = events.slice(i, i + MAX_EVENTS_PER_REQUEST);
      try {
        const result = await uploadBatch(chunk);
        accepted += result.accepted;
      } catch (err) {
        const remaining = events.slice(i);
        requeueEvents(remaining);
        if (__DEV__) {
          console.warn(
            `[notification-sync] upload failed at offset ${i}; requeued ${remaining.length} events`,
            err,
          );
        }
        return accepted;
      }
    }
    markSynced(new Date());
    return accepted;
  } finally {
    inflight = false;
  }
}

/**
 * startSync wires the foreground flush loop. Idempotent — calling twice
 * just resets the interval. The system suspends timers when the app
 * backgrounds, so we also flush on AppState "active" transitions to
 * cover the case of returning to foreground after an idle period.
 */
export function startSync(): void {
  if (!isCaptureAvailable) return;
  if (intervalHandle) {
    clearInterval(intervalHandle);
  }
  intervalHandle = setInterval(() => {
    void flushNow();
  }, FLUSH_INTERVAL_MS);

  if (appStateSubscription) {
    appStateSubscription.remove();
  }
  appStateSubscription = AppState.addEventListener(
    "change",
    (state: AppStateStatus) => {
      if (state === "active") {
        void flushNow();
      }
    },
  );
}

/**
 * stopSync tears down the timers and listeners. Safe to call from
 * unmount paths or when the user disables capture in settings.
 */
export function stopSync(): void {
  if (intervalHandle) {
    clearInterval(intervalHandle);
    intervalHandle = null;
  }
  if (appStateSubscription) {
    appStateSubscription.remove();
    appStateSubscription = null;
  }
}

/**
 * pendingCount is exported for the settings screen so the user can see
 * "X events waiting to upload". Reads from the native buffer directly.
 */
export function pendingCount(): number {
  return isCaptureAvailable ? bufferSize() : 0;
}
