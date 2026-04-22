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

