package service

import (
	"fmt"
	"html"
)

func otpSMS(app, code string, mins int) string {
	return fmt.Sprintf("%s: your verification code is %s. Valid for %d minutes. Never share this code with anyone.", app, code, mins)
}

func otpEmail(app, purpose, code string, mins int) (subject, text, htmlBody string) {
	action := map[string]string{
		purposeRegister: "finish creating your account",
		purposeLogin:    "sign in",
		purposeReset:    "reset your password",
		purposeContact:  "verify your email address",
	}[purpose]
	if action == "" {
		action = "continue"
	}
	subject = fmt.Sprintf("%s verification code: %s", app, code)
	text = fmt.Sprintf("Your %s verification code is %s\n\nUse it to %s. It expires in %d minutes.\n\n"+
		"If you did not request this, you can safely ignore this email. Never share this code with anyone.", app, code, action, mins)
	htmlBody = fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:480px;margin:auto;padding:24px">
<h2 style="color:#0f766e;margin:0 0 16px">%s</h2>
<p>Use this code to %s:</p>
<p style="font-size:32px;font-weight:bold;letter-spacing:8px;margin:24px 0">%s</p>
<p style="color:#555">This code expires in %d minutes.</p>
<p style="color:#888;font-size:12px">If you did not request this, ignore this email. Never share this code with anyone.</p>
</div>`, html.EscapeString(app), action, code, mins)
	return
}

func simpleEmail(app, title, body string) (subject, text, htmlBody string) {
	subject = app + ": " + title
	text = body
	htmlBody = fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:480px;margin:auto;padding:24px">
<h2 style="color:#0f766e;margin:0 0 16px">%s</h2><p>%s</p></div>`, html.EscapeString(title), html.EscapeString(body))
	return
}
