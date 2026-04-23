import { request } from "@/services/api";

export type AuthResponse = {
  token: string;
  user_id: string;
  email: string;
  name: string;
};

export type User = {
  id: string;
  identity_key: string;
  email: string;
  name: string;
  created_at: string;
  updated_at: string;
};

export async function sendMagicLink(email: string, inviteToken?: string) {
  const body: Record<string, string> = { email };
  if (inviteToken) {
    body.invite_token = inviteToken;
  }
  return request<{ status: string }>("/auth/magic-link", {
    method: "POST",
    auth: false,
    body: JSON.stringify(body)
  });
}

export type VerifyResponse = AuthResponse & {
  // invite_error is populated when the bundled invite_token failed to apply
  // (e.g. expired/revoked). The login itself still succeeded; the UI can
  // surface this to the user without blocking.
  invite_error?: string;
};

export async function verifyCode(email: string, code: string, inviteToken?: string) {
  const body: Record<string, string> = { email, code };
  if (inviteToken) {
    body.invite_token = inviteToken;
  }
  return request<VerifyResponse>("/auth/verify", {
    method: "POST",
    auth: false,
    body: JSON.stringify(body)
  });
}

export async function getMe() {
  // /api/me returns the caller identity and is unaffected by the active team.
  return request<User>("/api/me", { skipTeamHeader: true });
}

// devAutoLogin hits the unauthenticated dev login endpoint, which upserts the
// user (and personal team) for the given email and returns a real JWT. The
// AuthSessionProvider calls this on hydrate when EXPO_PUBLIC_DEV_AUTO_LOGIN
// is "true", so opening the app in local dev never requires entering an
// email or pasting a magic-link code. The token is identical in shape to one
// returned by verifyCode, so applyToken handles it without branching.
//
// LOCAL DEV ONLY — the endpoint exists in production builds too but is
// guarded by NODE_ENV checks on the server side; setting the env var in a
// shipped build would be visible in the bundle and is unsupported.
export async function devAutoLogin(email: string, name?: string) {
  return request<AuthResponse>("/auth/login", {
    method: "POST",
    auth: false,
    body: JSON.stringify({ email, name: name ?? "" })
  });
}

