import { API_URL } from "@/config";

type AccessTokenProvider = () => Promise<string | null>;
type ActiveTeamProvider = () => string | null;

type RequestOptions = RequestInit & {
  auth?: boolean;
  // skipTeamHeader opts out of the X-Team-ID header for endpoints that should
  // act on the user identity globally (e.g. /api/me, /api/teams/* listing).
  skipTeamHeader?: boolean;
};

let accessTokenProvider: AccessTokenProvider | null = null;
let activeTeamProvider: ActiveTeamProvider | null = null;

export class APIError extends Error {
  status: number;
  body: string;

  constructor(status: number, body: string) {
    super(body || `Request failed with status ${status}`);
    this.name = "APIError";
    this.status = status;
    this.body = body;
  }
}

export class AuthenticationError extends APIError {
  constructor(status: number, body: string) {
    super(status, body);
    this.name = "AuthenticationError";
  }
}

export function isAuthenticationError(error: unknown): error is AuthenticationError {
  return error instanceof AuthenticationError;
}

export function setAccessTokenProvider(provider: AccessTokenProvider | null) {
  accessTokenProvider = provider;
}

export async function getAccessToken(): Promise<string | null> {
  if (!accessTokenProvider) {
    return null;
  }
  return accessTokenProvider();
}

// setActiveTeamProvider lets the TeamsProvider register a callback that returns
// the currently-selected team id. The api layer reads it on every request and
// stamps X-Team-ID so backend agent/teams/invites endpoints scope correctly.
export function setActiveTeamProvider(provider: ActiveTeamProvider | null) {
  activeTeamProvider = provider;
}

export function getActiveTeamID(): string | null {
  return activeTeamProvider ? activeTeamProvider() : null;
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { auth = true, skipTeamHeader = false, headers, ...rest } = options;
  const resolvedHeaders: Record<string, string> = {
    "Content-Type": "application/json",
    ...(headers as Record<string, string> | undefined)
  };

  if (auth) {
    if (!accessTokenProvider) {
      throw new AuthenticationError(401, "Authentication provider is not configured");
    }
    const token = await accessTokenProvider();
    if (!token) {
      throw new AuthenticationError(401, "Missing access token");
    }
    resolvedHeaders.Authorization = `Bearer ${token}`;
  }

  // Stamp the active team on auth'd calls so backend handlers that go through
  // RequireTeam know which team this request is for. Skipped on:
  //   - unauthenticated calls (no team context to share),
  //   - explicit skipTeamHeader (e.g. /api/me, /api/teams/* listing where the
  //     server resolves team independently).
  if (auth && !skipTeamHeader) {
    const team = getActiveTeamID();
    if (team) {
      resolvedHeaders["X-Team-ID"] = team;
    }
  }

  const response = await fetch(`${API_URL.replace(/\/+$/, "")}${path}`, {
    ...rest,
    headers: resolvedHeaders
  });

  if (!response.ok) {
    const body = await response.text();
    if (response.status === 401 || response.status === 403) {
      throw new AuthenticationError(response.status, body);
    }
    throw new APIError(response.status, body);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return (await response.text()) as T;
  }
  return (await response.json()) as T;
}
