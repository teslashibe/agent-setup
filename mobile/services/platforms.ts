import { request } from "@/services/api";

export type PlatformConnectionSummary = {
  platform: string;
  label?: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string | null;
};

export type PlatformStatus = {
  platform: string;
  connected: boolean;
  summary?: PlatformConnectionSummary;
};

export type PlatformListResponse = {
  platforms: PlatformStatus[];
};

/** Lists every platform agent-setup knows about, marking which the
 * authenticated user has already connected. */
export async function listPlatforms(): Promise<PlatformStatus[]> {
  const res = await request<PlatformListResponse>("/api/platforms");
  return res.platforms;
}

/** Stores or replaces the credential for `platform`. The shape of
 * `credential` is intentionally a free-form JSON blob so each platform's
 * UI can collect whatever fields it needs (single cookie, cookies map,
 * API token, etc.). The backend AES-GCM-encrypts before persisting. */
export async function setPlatformCredential(
  platform: string,
  credential: Record<string, unknown>,
  label?: string
): Promise<PlatformConnectionSummary> {
  return request<PlatformConnectionSummary>(`/api/platforms/${encodeURIComponent(platform)}/credentials`, {
    method: "POST",
    body: JSON.stringify({ credential, label: label ?? "" })
  });
}

/** Disconnects (deletes the stored credential for) `platform`. */
export async function disconnectPlatform(platform: string): Promise<void> {
  await request<void>(`/api/platforms/${encodeURIComponent(platform)}/credentials`, {
    method: "DELETE"
  });
}

/** Static metadata about a platform — what it's called, which fields the
 * settings UI should ask for, and the canonical Chrome cookie-extractor
 * URL (a one-line link in the connect modal so users know where to go).
 *
 * The list mirrors `platforms.All()` in the backend. Order here is the
 * order shown in the UI before any are connected. */
export type PlatformMetadata = {
  id: string;
  name: string;
  /** Short helper text shown above the credential textarea. */
  helper: string;
  /** Each field the settings UI prompts for. Most platforms only need
   * one (cookie/token); multi-cookie platforms (Facebook, Instagram,
   * TikTok) need several. */
  fields: PlatformField[];
  /** When true the platform doesn't need credentials (deterministic
   * scorers, locally-shelled CLIs). */
  noCredentials?: boolean;
};

export type PlatformField = {
  name: string;
  label: string;
  helper?: string;
  /** Map this raw field into the credential blob the backend expects.
   * If `kind === "cookie"` the value is wrapped as
   * `{cookies: {<name>: value}}`; if `kind === "token"` it goes into
   * `{token: value}`; if `kind === "extra"` it is forwarded under
   * `{extra: {<name>: value}}`. */
  kind: "cookie" | "token" | "extra";
  placeholder?: string;
};

const COOKIE_EXTENSION = "https://chromewebstore.google.com/detail/cookie-editor/hlkenndednhfkekhgcdicdfddnkalmdm";

export const PLATFORMS: PlatformMetadata[] = [
  {
    id: "linkedin",
    name: "LinkedIn",
    helper: `Open LinkedIn while signed in, copy the li_at and JSESSIONID cookies via the cookie editor extension.`,
    fields: [
      { name: "li_at", label: "li_at cookie", kind: "cookie" },
      { name: "JSESSIONID", label: "JSESSIONID cookie", kind: "cookie" }
    ]
  },
  {
    id: "x",
    name: "X (Twitter)",
    helper: "From x.com (signed in): copy the auth_token and ct0 cookies.",
    fields: [
      { name: "auth_token", label: "auth_token cookie", kind: "cookie" },
      { name: "ct0", label: "ct0 cookie", kind: "cookie" },
      { name: "twid", label: "twid cookie (optional)", kind: "cookie" }
    ]
  },
  {
    id: "xviral",
    name: "X Viral Scoring",
    helper: "Deterministic local scorer — no credentials required.",
    fields: [],
    noCredentials: true
  },
  {
    id: "reddit",
    name: "Reddit",
    helper: "Signed-in reddit.com → DevTools → Application → Cookies → token_v2.",
    fields: [
      { name: "token_v2", label: "token_v2 cookie", kind: "cookie" }
    ]
  },
  {
    id: "redditviral",
    name: "Reddit Viral Scoring",
    helper: "Deterministic local scorer — no credentials required.",
    fields: [],
    noCredentials: true
  },
  {
    id: "hn",
    name: "Hacker News",
    helper: "Signed-in news.ycombinator.com → user cookie value.",
    fields: [
      { name: "user", label: "user cookie", kind: "cookie" }
    ]
  },
  {
    id: "facebook",
    name: "Facebook (Groups)",
    helper: "From facebook.com signed in, copy c_user, xs, fr, datr, sb cookies.",
    fields: [
      { name: "c_user", label: "c_user cookie", kind: "cookie" },
      { name: "xs", label: "xs cookie", kind: "cookie" },
      { name: "fr", label: "fr cookie", kind: "cookie" },
      { name: "datr", label: "datr cookie", kind: "cookie" },
      { name: "sb", label: "sb cookie (optional)", kind: "cookie" }
    ]
  },
  {
    id: "instagram",
    name: "Instagram",
    helper: "From instagram.com signed in, copy sessionid, csrftoken, ds_user_id, datr.",
    fields: [
      { name: "sessionid", label: "sessionid cookie", kind: "cookie" },
      { name: "csrftoken", label: "csrftoken cookie", kind: "cookie" },
      { name: "ds_user_id", label: "ds_user_id cookie", kind: "cookie" },
      { name: "datr", label: "datr cookie (optional)", kind: "cookie" }
    ]
  },
  {
    id: "tiktok",
    name: "TikTok",
    helper: "From tiktok.com signed in, copy sessionid, tt_csrf_token, msToken cookies.",
    fields: [
      { name: "sessionid", label: "sessionid cookie", kind: "cookie" },
      { name: "tt_csrf_token", label: "tt_csrf_token cookie", kind: "cookie" },
      { name: "msToken", label: "msToken cookie", kind: "cookie" }
    ]
  },
  {
    id: "threads",
    name: "Threads",
    helper:
      "Read access: threads.com/instagram cookies (sessionid, csrftoken). Write access also needs a Bearer token (IGT:2:…) and your numeric user_id (paste those in Optional fields).",
    fields: [
      { name: "sessionid", label: "sessionid cookie", kind: "cookie" },
      { name: "csrftoken", label: "csrftoken cookie", kind: "cookie" },
      { name: "ds_user_id", label: "ds_user_id cookie", kind: "cookie" },
      { name: "bearer", label: "Bearer token (optional, write)", kind: "extra" },
      { name: "user_id", label: "Numeric user_id (optional, write)", kind: "extra" }
    ]
  },
  {
    id: "producthunt",
    name: "Product Hunt",
    helper: "Easiest path: paste a Product Hunt v2 developer token (settings → API).",
    fields: [
      { name: "developer_token", label: "Developer token (BYOK)", kind: "token" }
    ]
  },
  {
    id: "nextdoor",
    name: "Nextdoor",
    helper: "From nextdoor.com signed in, copy the access_token cookie and xsrf token.",
    fields: [
      { name: "access_token", label: "access_token cookie", kind: "cookie" },
      { name: "xsrf", label: "xsrf token", kind: "cookie" }
    ]
  },
  {
    id: "elevenlabs",
    name: "ElevenLabs",
    helper: "ElevenLabs settings → Profile → API key (XI-API-Key).",
    fields: [
      { name: "api_key", label: "XI-API-Key", kind: "token" }
    ]
  },
  {
    id: "codegen",
    name: "Codegen (Claude Code)",
    helper: "Runs locally against the `claude` CLI installed on the API host. No credential required.",
    fields: [],
    noCredentials: true
  }
];

export const COOKIE_EDITOR_LINK = COOKIE_EXTENSION;

/** Builds the credential JSON blob the backend expects from raw form
 * field values. */
export function buildCredential(
  fields: PlatformField[],
  values: Record<string, string>
): Record<string, unknown> {
  const cookies: Record<string, string> = {};
  const extra: Record<string, string> = {};
  let token: string | undefined;
  for (const f of fields) {
    const v = values[f.name]?.trim();
    if (!v) continue;
    if (f.kind === "cookie") cookies[f.name] = v;
    else if (f.kind === "token") token = v;
    else extra[f.name] = v;
  }
  const out: Record<string, unknown> = {};
  if (Object.keys(cookies).length > 0) out.cookies = cookies;
  if (Object.keys(extra).length > 0) out.extra = extra;
  if (token) out.token = token;
  return out;
}

/** Parses what the user pasted into the "extension JSON / cookie string"
 * textarea and returns a normalised credential blob the backend will accept.
 *
 * Three accepted shapes (auto-detected):
 *
 * 1. Cookie-Editor JSON export — an array of objects each with at least
 *    `name` and `value` fields. We keep only fields whose name is in
 *    `meta.fields` (so we don't blow up the encrypted blob with site-wide
 *    cookies the scraper doesn't need). When `meta.fields` is empty we
 *    keep everything.
 * 2. A raw HTTP `Cookie:` header — semicolon-separated `name=value` pairs.
 *    We parse and filter the same way.
 * 3. A bare JSON object — used as-is (advanced users can paste exactly
 *    what the backend expects).
 *
 * Throws on un-parseable input so the caller can surface a clear error. */
export function parseExtensionInput(meta: PlatformMetadata, raw: string): Record<string, unknown> {
  const trimmed = raw.trim();
  if (!trimmed) throw new Error("Paste the cookie JSON or string first");

  const wantedNames = new Set(meta.fields.filter((f) => f.kind === "cookie").map((f) => f.name));
  const keepCookie = (name: string): boolean => wantedNames.size === 0 || wantedNames.has(name);

  if (trimmed.startsWith("[") || trimmed.startsWith("{")) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(trimmed);
    } catch (e) {
      throw new Error(`Could not parse JSON: ${e instanceof Error ? e.message : "unknown"}`);
    }
    if (Array.isArray(parsed)) {
      const cookies: Record<string, string> = {};
      for (const item of parsed) {
        if (!item || typeof item !== "object") continue;
        const name = (item as Record<string, unknown>).name;
        const value = (item as Record<string, unknown>).value;
        if (typeof name === "string" && typeof value === "string" && keepCookie(name)) {
          cookies[name] = value;
        }
      }
      if (Object.keys(cookies).length === 0) {
        throw new Error(
          `Pasted JSON didn't contain any of: ${Array.from(wantedNames).join(", ") || "name/value entries"}`
        );
      }
      return { cookies };
    }
    if (typeof parsed === "object" && parsed) {
      return parsed as Record<string, unknown>;
    }
    throw new Error("Pasted JSON must be either an array (Cookie-Editor) or an object");
  }

  const cookies: Record<string, string> = {};
  for (const part of trimmed.split(";")) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    const name = part.slice(0, eq).trim();
    const value = part.slice(eq + 1).trim();
    if (name && keepCookie(name)) cookies[name] = value;
  }
  if (Object.keys(cookies).length === 0) {
    throw new Error(
      `No cookies matched. Expected name=value pairs (e.g. ${Array.from(wantedNames).join(", ") || "li_at=…"})`
    );
  }
  return { cookies };
}
