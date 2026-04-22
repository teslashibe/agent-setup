import { fetch as expoFetch } from "expo/fetch";

import { API_URL } from "@/config";
import { getAccessToken, getActiveTeamID, request } from "@/services/api";

export type Session = {
  id: string;
  team_id: string;
  user_id: string;
  title: string;
  anthropic_session_id?: string;
  system_prompt?: string | null;
  model?: string | null;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type ListScope = "mine" | "all";

export type Message = {
  id: string;
  session_id: string;
  role: "user" | "assistant";
  content: unknown;
  stop_reason?: string | null;
  input_tokens?: number | null;
  output_tokens?: number | null;
  created_at: string;
};

export type AgentEvent =
  | { type: "text"; text: string }
  | { type: "tool_use"; tool: string; tool_id: string; input?: unknown }
  | { type: "tool_result"; tool: string; tool_id: string; output?: unknown; is_error?: boolean }
  | { type: "usage"; usage: { input_tokens: number; output_tokens: number } }
  | { type: "done" }
  | { type: "error"; error: string };

// listSessions defaults to ?scope=mine; pass "all" only when the caller is at
// least admin in the active team (the backend enforces this with a 403).
export async function listSessions(scope: ListScope = "mine") {
  const res = await request<{ sessions: Session[]; scope: ListScope }>(
    `/api/agent/sessions?scope=${scope}`,
  );
  return res.sessions ?? [];
}

export async function createSession(title: string, opts?: { systemPrompt?: string; model?: string }) {
  return request<Session>("/api/agent/sessions", {
    method: "POST",
    body: JSON.stringify({
      title,
      system_prompt: opts?.systemPrompt,
      model: opts?.model
    })
  });
}

export async function listMessages(sessionId: string) {
  const res = await request<{ messages: Message[] }>(`/api/agent/sessions/${sessionId}/messages`);
  return res.messages ?? [];
}

// runSession streams Server-Sent Events from the backend agent loop.
// Uses expo/fetch which supports streaming responses on iOS, Android, and web.
export async function* runSession(
  sessionId: string,
  message: string,
  signal?: AbortSignal
): AsyncGenerator<AgentEvent, void, unknown> {
  const token = await getAccessToken();
  if (!token) {
    throw new Error("Not authenticated");
  }

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
    Accept: "text/event-stream",
  };
  const team = getActiveTeamID();
  if (team) {
    headers["X-Team-ID"] = team;
  }

  const response = await expoFetch(
    `${API_URL.replace(/\/+$/, "")}/api/agent/sessions/${sessionId}/run`,
    {
      method: "POST",
      headers,
      body: JSON.stringify({ message }),
      signal
    }
  );

  if (!response.ok || !response.body) {
    const text = await response.text().catch(() => "");
    throw new Error(`Agent run failed (${response.status}): ${text}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const rawEvent = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      const dataLines = rawEvent
        .split("\n")
        .filter((line) => line.startsWith("data: "))
        .map((line) => line.slice(6));
      if (dataLines.length === 0) continue;
      const json = dataLines.join("\n");
      try {
        yield JSON.parse(json) as AgentEvent;
      } catch {
        // Ignore malformed events; the stream will continue.
      }
    }
  }
}
