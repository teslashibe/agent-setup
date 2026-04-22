# Platform credentials setup

The MCP layer wraps 13 third-party platforms (LinkedIn, X, Reddit, etc.).
Each user pastes their own credentials in **Settings → Platform Connections**;
the backend AES-GCM-encrypts them in the `platform_credentials` table and
hands them to the matching scraper at request time. No credentials ever
leave your deployment.

This document covers:

1. Operator setup (env vars, DB migration, key rotation).
2. End-user setup per platform (where to find each cookie/token).
3. API surface (`/api/platforms/...`).

---

## 1. Operator setup

### 1.1 Encryption key

Generate a 32-byte key once per environment and put it in your secret
store:

```bash
openssl rand -hex 32
```

Then export:

```bash
export CREDENTIALS_ENCRYPTION_KEY=<hex>
```

In Kubernetes:

```yaml
env:
  - name: CREDENTIALS_ENCRYPTION_KEY
    valueFrom:
      secretKeyRef:
        name: agent-setup
        key: credentials_encryption_key
```

### 1.2 Migration

The `platform_credentials` table is created by the standard `migrate`
goose chain (`backend/migrations/...`). No manual step required —
`make dev-all` runs it automatically.

### 1.3 Key rotation

To rotate the key:

1. Generate a new key.
2. Run `cmd/credentials-rekey OLD_KEY NEW_KEY` (TODO: implement during
   Issue #6 follow-up).
3. Replace the env var.
4. Restart the API pods.

Until the rekey CLI ships, key rotation requires asking users to
re-paste their credentials. Don't rotate routinely; rotate on incident.

---

## 2. End-user setup

The settings UI explains each platform inline, but here's the canonical
reference. The recommended cookie-extraction tool is
[Cookie-Editor](https://chromewebstore.google.com/detail/cookie-editor/hlkenndednhfkekhgcdicdfddnkalmdm).

| Platform | Auth | What to paste |
|---|---|---|
| **LinkedIn** | cookie | `li_at` + `JSESSIONID` from `linkedin.com` (signed in) |
| **X / Twitter** | cookie | `auth_token` + `ct0` (+ optional `twid`) from `x.com` |
| **X Viral Scoring** | none | Deterministic local scorer, no credentials |
| **Reddit** | cookie | `token_v2` from `reddit.com` (signed in) |
| **Reddit Viral Scoring** | none | Deterministic local scorer, no credentials |
| **Hacker News** | cookie | `user` cookie from `news.ycombinator.com` |
| **Facebook** | cookie | `c_user`, `xs`, `fr`, `datr` (+ optional `sb`) |
| **Instagram** | cookie | `sessionid`, `csrftoken`, `ds_user_id` (+ optional `datr`) |
| **TikTok** | cookie | `sessionid`, `tt_csrf_token`, `msToken` |
| **Product Hunt** | token | Developer token from PH settings → API |
| **Nextdoor** | cookie | `access_token` cookie + `xsrf` token |
| **ElevenLabs** | API key | `XI-API-Key` from ElevenLabs profile |
| **Codegen (Claude Code)** | none | Local `claude` CLI; nothing to paste |

### How to extract a cookie (LinkedIn example)

1. Sign in to `https://linkedin.com` in Chrome.
2. Click the Cookie-Editor extension icon.
3. Find `li_at` → "Copy → Copy Value".
4. In Engagement Studio: **Settings → Platform Connections → LinkedIn → Connect**, paste it.
5. Repeat for `JSESSIONID`.
6. Hit **Save credentials**.

The frontend wraps your raw values into the canonical credential blob the
backend expects, so you never need to hand-craft JSON.

---

## 3. API surface

Routes are mounted under `/api/platforms` and require a magic-link bearer
token in `Authorization: Bearer <jwt>`.

### `GET /api/platforms`

Lists every platform `agent-setup` knows about, plus whether the caller
has a stored credential.

```json
{
  "platforms": [
    {
      "platform": "linkedin",
      "connected": true,
      "summary": {
        "platform": "linkedin",
        "label": "Personal account",
        "created_at": "2026-04-22T20:30:00Z",
        "updated_at": "2026-04-22T20:31:00Z",
        "last_used_at": "2026-04-22T20:45:00Z"
      }
    },
    { "platform": "x", "connected": false }
  ]
}
```

### `GET /api/platforms/:platform/credentials`

Returns connection metadata only — the credential blob itself is **never**
returned to clients. 404 if not connected.

### `POST /api/platforms/:platform/credentials` (or `PUT`)

Upserts a credential. Body:

```json
{
  "credential": { "cookies": { "li_at": "AQED...", "JSESSIONID": "ajax:..." } },
  "label": "Personal account"
}
```

The shape of `credential` is platform-specific:

- **cookie-only platforms** — `{"cookies": {"<name>": "<value>", ...}}`
- **token platforms** — `{"token": "<value>"}`
- **mixed** — `{"cookies": {...}, "token": "...", "extra": {...}}`

The frontend builds this blob automatically; if you're calling the API
directly, see `mobile/services/platforms.ts → buildCredential`.

The handler runs the platform's `Validator` (e.g., LinkedIn requires
`li_at` and `JSESSIONID`) before encrypting and persisting.

### `DELETE /api/platforms/:platform/credentials`

Removes the stored credential and returns 204.

---

## 4. Security notes

- Credentials are encrypted at rest with AES-GCM (`credentials.Cipher`).
- The plaintext blob exists only in-memory while building a per-request
  scraper client; it is never logged.
- Per-tool-call observability records `platform`, never the credential.
- The path-mounted MCP JWT (`/mcp/u/<token>/v1`) is short-lived (24h) and
  scoped to a single subject. If a JWT leaks, only that user's stored
  credentials are exposed — and only via the tool surface, never as the
  raw blob.
