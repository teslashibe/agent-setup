package invites

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// EmailSender is the minimal email-sending contract this package needs.
// It deliberately matches magiclink-auth-go's resend.Sender so that sender
// can be reused without an adapter.
type EmailSender interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// EmailRequest captures everything renderEmail needs.
type EmailRequest struct {
	AppName       string
	FromName      string
	InviterEmail  string
	InviterName   string
	TeamName      string
	Role          string
	AcceptURL     string
	ExpiresInDays int
}

// renderInviteEmail returns subject + HTML body for an invite email.
func renderInviteEmail(req EmailRequest) (string, string) {
	subject := fmt.Sprintf("%s invited you to join %s on %s",
		coalesce(req.InviterName, req.InviterEmail),
		req.TeamName,
		req.AppName,
	)

	body := fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f5f5f7;padding:24px;color:#111;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="max-width:560px;margin:0 auto;background:white;border-radius:12px;border:1px solid #e5e5ea;">
    <tr><td style="padding:32px 32px 16px;">
      <p style="margin:0;font-size:14px;color:#6b6b73;">%s</p>
      <h1 style="margin:8px 0 16px;font-size:22px;line-height:1.3;">You're invited to join <strong>%s</strong></h1>
      <p style="margin:0 0 16px;font-size:15px;line-height:1.5;">
        %s (<a href="mailto:%s" style="color:#007aff;text-decoration:none;">%s</a>) invited you to join
        <strong>%s</strong> as a <strong>%s</strong>.
      </p>
      <p style="margin:0 0 24px;font-size:15px;line-height:1.5;">
        Click the button below to accept. This invite expires in %d days.
      </p>
      <p style="margin:0 0 32px;">
        <a href="%s" style="display:inline-block;background:#007aff;color:white;text-decoration:none;padding:12px 20px;border-radius:8px;font-size:15px;font-weight:600;">Accept invite</a>
      </p>
      <p style="margin:0;font-size:13px;color:#6b6b73;">
        Or copy this link: <span style="word-break:break-all;color:#3c3c43;">%s</span>
      </p>
    </td></tr>
    <tr><td style="padding:16px 32px 24px;border-top:1px solid #f0f0f3;font-size:12px;color:#8e8e93;">
      Sent by %s. If you weren't expecting this invite you can safely ignore it.
    </td></tr>
  </table>
</body></html>`,
		html.EscapeString(subject),
		html.EscapeString(coalesce(req.FromName, req.AppName)),
		html.EscapeString(req.TeamName),
		html.EscapeString(coalesce(req.InviterName, req.InviterEmail)),
		html.EscapeString(req.InviterEmail),
		html.EscapeString(req.InviterEmail),
		html.EscapeString(req.TeamName),
		html.EscapeString(strings.ToLower(req.Role)),
		req.ExpiresInDays,
		html.EscapeString(req.AcceptURL),
		html.EscapeString(req.AcceptURL),
		html.EscapeString(coalesce(req.FromName, req.AppName)),
	)

	return subject, body
}

func coalesce(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// devLogger is used when no real EmailSender is configured. It dumps every
// invite to stderr so the link is visible during local development.
type devLogger struct{}

func (devLogger) Send(_ context.Context, to, subject, htmlBody string) error {
	fmt.Printf("\n[invite-dev-email] to=%s subject=%q\n%s\n\n", to, subject, htmlBody)
	return nil
}
