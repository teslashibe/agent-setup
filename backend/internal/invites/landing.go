package invites

import (
	"errors"
	"fmt"
	"html"
	"net/url"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

func landingHTML(p Preview, token, mobileScheme string) string {
	deepLink := ""
	if mobileScheme != "" {
		deepLink = mobileScheme + "://invites/accept?token=" + url.QueryEscape(token)
	}
	autoRedirect := ""
	if deepLink != "" {
		autoRedirect = fmt.Sprintf(`<meta http-equiv="refresh" content="0;url=%s">
<script>window.location.replace(%q);</script>`, html.EscapeString(deepLink), deepLink)
	}
	openAppButton := ""
	if deepLink != "" {
		openAppButton = fmt.Sprintf(`<p style="margin:24px 0;"><a href="%s" style="display:inline-block;background:#007aff;color:white;text-decoration:none;padding:12px 20px;border-radius:8px;font-size:15px;font-weight:600;">Open in app</a></p>`, html.EscapeString(deepLink))
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Invite to %s</title>%s</head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f5f5f7;color:#111;padding:24px;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;">
<div style="max-width:480px;background:white;border-radius:16px;border:1px solid #e5e5ea;padding:32px;text-align:center;">
  <p style="margin:0 0 8px;font-size:13px;color:#6b6b73;text-transform:uppercase;letter-spacing:.06em;">Invite</p>
  <h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;">You've been invited to <strong>%s</strong></h1>
  <p style="margin:0 0 8px;font-size:15px;color:#3c3c43;">As <strong>%s</strong>, sent to %s.</p>
  %s
  <p style="margin:0;font-size:13px;color:#8e8e93;">If the app doesn't open automatically, sign in with the email above to accept.</p>
</div></body></html>`,
		html.EscapeString(p.Team.Name),
		autoRedirect,
		html.EscapeString(p.Team.Name),
		html.EscapeString(string(p.Role)),
		html.EscapeString(p.Email),
		openAppButton,
	)
}

func landingErrorHTML(err error) string {
	msg := "We couldn't process this invite."
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		msg = appErr.Message
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>Invite unavailable</title></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;padding:24px;color:#111;text-align:center;">
<h1 style="margin:0 0 16px;">Invite unavailable</h1>
<p style="margin:0;color:#3c3c43;">%s</p>
</body></html>`, html.EscapeString(msg))
}
