import { request } from "@/services/api";

import type { CapturedNotification } from "../modules/notification-capture/src";

export type { CapturedNotification };

export interface IngestResult {
  accepted: number;
}

export interface AppSummary {
  app_package: string;
  app_label: string;
  count: number;
  last_at: string;
}

/**
 * uploadBatch ships a freshly drained buffer to the backend. Returns the
 * number of rows the server actually accepted (post-deduplication).
 *
 * The shape is intentionally minimal: the native side already produces
 * payloads matching the backend's `notifications.EventInput` JSON schema,
 * so this function is a thin POST and nothing more.
 */
export async function uploadBatch(events: CapturedNotification[]): Promise<IngestResult> {
  if (events.length === 0) {
    return { accepted: 0 };
  }
  return request<IngestResult>("/api/notifications/batch", {
    method: "POST",
    body: JSON.stringify({ events }),
    skipTeamHeader: true,
  });
}

/**
 * listCapturedApps powers the settings screen's "X apps captured" stat.
 * Calls the same handler the agent's notifications_apps MCP tool wraps so
 * the two views can never diverge.
 */
export async function listCapturedApps(): Promise<AppSummary[]> {
  const res = await request<{ apps: AppSummary[]; count: number }>(
    "/api/notifications/apps",
    { skipTeamHeader: true },
  );
  return res.apps ?? [];
}
