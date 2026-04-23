import { API_URL } from "@/config";
import {
  APIError,
  AuthenticationError,
  getAccessToken,
  getActiveTeamID
} from "@/services/api";

// uploadAttachment posts a single image to the backend's
// `/api/uploads` endpoint as multipart/form-data. The backend stores
// the blob and returns a signed, short-lived URL the chat composer
// can embed in the user's message and the agent can fetch from its
// MCP tool.
//
// We deliberately do NOT route this through `services/api.ts → request()`:
// `request()` always sets `Content-Type: application/json`, but multipart
// uploads need the `Content-Type` header to be set by the runtime so the
// `boundary=` token gets generated. Letting fetch fill it in is the
// simplest reliable path on iOS, Android, and web.

export type UploadedAttachment = {
  id: string;
  url: string;
  mime_type: string;
  size: number;
  original_name?: string;
};

export type AttachmentInput = {
  // file:// or content:// URI returned by expo-image-picker (or a
  // blob: URL on web). Read by FormData under the hood — we never
  // load the bytes into JS, which keeps the picker → upload path
  // light even on a 10 MB photo.
  uri: string;
  // Display name shown in the picker / used as Content-Disposition
  // when the file lands on disk. Falls back to a synthetic name if
  // the picker didn't supply one (Android often doesn't).
  name?: string;
  // image/jpeg, image/png, etc. The backend rejects anything that
  // isn't an `image/*` MIME, so the picker should stay locked to
  // images-only.
  mimeType?: string;
};

/**
 * uploadAttachment uploads a single picked image to the backend.
 *
 * Throws `AuthenticationError` if the user has no access token, or
 * `APIError` for any non-2xx response (including the backend's 413
 * for files >10 MiB and 400 for non-image MIME types).
 */
export async function uploadAttachment(input: AttachmentInput): Promise<UploadedAttachment> {
  const token = await getAccessToken();
  if (!token) {
    throw new AuthenticationError(401, "Missing access token");
  }

  const name = input.name?.trim() || synthName(input.mimeType);
  const type = input.mimeType?.trim() || guessMime(name);

  // React Native's FormData accepts the `{ uri, name, type }` triple
  // as a "file" part — the runtime streams the file from `uri`
  // without loading the bytes into JS. On web the same shape is
  // built into expo's FormData polyfill via Blob.
  const form = new FormData();
  // The backend handler reads `c.FormFile("file")`, so the field
  // name MUST be exactly "file".
  form.append("file", {
    uri: input.uri,
    name,
    type
    // RN's FormData type doesn't model `{ uri }` parts in TS, but
    // it's the documented escape hatch for streaming uploads.
  } as unknown as Blob);

  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    Accept: "application/json"
    // NOTE: Do not set Content-Type — fetch needs to add the
    // multipart boundary itself.
  };
  const team = getActiveTeamID();
  if (team) {
    headers["X-Team-ID"] = team;
  }

  const response = await fetch(`${API_URL.replace(/\/+$/, "")}/api/uploads`, {
    method: "POST",
    headers,
    body: form
  });

  if (!response.ok) {
    const body = await response.text().catch(() => "");
    if (response.status === 401 || response.status === 403) {
      throw new AuthenticationError(response.status, body);
    }
    throw new APIError(response.status, body);
  }

  return (await response.json()) as UploadedAttachment;
}

// synthName produces a stable, unique filename when the picker
// doesn't supply one. The backend's hosted endpoints rarely care
// about the uploaded filename — they derive their asset URL from the
// upload id — but a meaningful extension helps the backend's MIME
// sniff and gives the operator something to recognize in the chat
// history.
function synthName(mimeType?: string): string {
  const ext = mimeToExt(mimeType);
  const stamp = new Date().toISOString().replace(/[-:.]/g, "").slice(0, 15);
  return `attachment-${stamp}.${ext}`;
}

function mimeToExt(mimeType?: string): string {
  switch (mimeType) {
    case "image/jpeg":
    case "image/jpg":
      return "jpg";
    case "image/png":
      return "png";
    case "image/gif":
      return "gif";
    case "image/webp":
      return "webp";
    case "image/heic":
      return "heic";
    case "image/heif":
      return "heif";
    default:
      // Most pickers default to JPEG — safer than `bin`.
      return "jpg";
  }
}

function guessMime(name: string): string {
  const lower = name.toLowerCase();
  if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) return "image/jpeg";
  if (lower.endsWith(".png")) return "image/png";
  if (lower.endsWith(".gif")) return "image/gif";
  if (lower.endsWith(".webp")) return "image/webp";
  if (lower.endsWith(".heic")) return "image/heic";
  if (lower.endsWith(".heif")) return "image/heif";
  return "application/octet-stream";
}
