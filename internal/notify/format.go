package notify

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"
	"time"
)

// Stat is one uptime figure shown in an alert.
type Stat struct {
	Label   string
	Percent float64
}

// palette is what a mail needs from a theme. The values are the theme's in web/src/routes/layout.css; a test keeps them equal.
type palette struct{ Canvas, Surface, Border, Text, Muted, Accent, AccentText string }

var palettes = map[string]palette{
	"graphite": {"#08090b", "#13161a", "#2b323b", "#f2f5f7", "#9da6b2", "#2563eb", "#ffffff"},
	"linen":    {"#f3eee5", "#faf7f1", "#d4c7b6", "#2b251f", "#74695d", "#b6533a", "#ffffff"},
	"verdant":  {"#030a07", "#0a1a12", "#27513b", "#edf7f0", "#94aa9b", "#e8ce55", "#08110c"},
	"violet":   {"#05030a", "#110c1d", "#382653", "#f4f0fa", "#aa9fbc", "#e6cf62", "#120c1e"},
}

// stateColors are fixed across themes (10-ui.md §2).
var stateColors = map[string]string{"up": "#22C55E", "degraded": "#F59E0B", "down": "#EF4444", "test": "#3B82F6"}

func stateEmoji(state string) string {
	switch state {
	case "up":
		return "🟢"
	case "degraded":
		return "🟡"
	case "down":
		return "🔴"
	case "test":
		return "🔔"
	}
	return ""
}

func percent(v float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 2, 64), "0"), ".") + " %"
}

func statLine(stats []Stat) string {
	parts := make([]string, len(stats))
	for i, s := range stats {
		parts[i] = s.Label + ": " + percent(s.Percent)
	}
	return strings.Join(parts, " · ")
}

func timeLine(m Message) string {
	if m.At.IsZero() {
		return ""
	}
	return m.At.UTC().Format("2 Jan 2006, 15:04") + " UTC"
}

// plainDuration says how long an outage lasted, in words.
func plainDuration(d time.Duration) string {
	unit := func(n int, name string) string {
		if n == 1 {
			return "1 " + name
		}
		return strconv.Itoa(n) + " " + name + "s"
	}
	switch {
	case d < time.Minute:
		return unit(max(int(d.Seconds()), 1), "second")
	case d < time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return unit(int(d.Hours()), "hour") + " " + unit(int(d.Minutes())%60, "minute")
	}
	return unit(int(d.Hours())/24, "day") + " " + unit(int(d.Hours())%24, "hour")
}

// plainReason turns a check's raw error into a sentence safe to hand to a mail or chat provider. The raw text names
// internal addresses and stays in Hashi's own event history.
func plainReason(raw string) string {
	r := strings.ToLower(raw)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(r, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("latency"):
		return "It is answering slower than its limit."
	case has("connection refused"):
		return "The service refused the connection."
	case has("deadline exceeded", "timeout", "timed out"):
		return "The service did not answer in time."
	case has("no such host", "no answer", "answer "):
		return "Its DNS name did not resolve as expected."
	case has("certificate", "x509", "tls"):
		return "Its certificate or secure connection failed."
	case has("unreachable", "no route"):
		return "The service cannot be reached over the network."
	case has("http "):
		return "The service answered with an error."
	}
	return "Its check failed."
}

// telegramHTML is Telegram's HTML message style (testdata/contracts/telegram/sendMessage-html.json).
func telegramHTML(m Message) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(stateEmojiPrefix(m) + "<b>" + e(m.Subject) + "</b>")
	if m.Body != "" {
		b.WriteString("\n" + e(m.Body))
	}
	if t := timeLine(m); t != "" {
		b.WriteString("\n<i>" + e(t) + "</i>")
	}
	if len(m.Stats) > 0 {
		b.WriteString("\n\nUptime " + e(statLine(m.Stats)))
	}
	if m.Link != "" {
		b.WriteString("\n\n<a href=\"" + e(m.Link) + "\">Open in Hashi</a>")
	}
	return b.String()
}

func stateEmojiPrefix(m Message) string {
	if e := stateEmoji(m.State); e != "" {
		return e + " "
	}
	return ""
}

func mailText(m Message) string {
	lines := []string{m.Subject}
	if m.Body != "" {
		lines = append(lines, m.Body)
	}
	if t := timeLine(m); t != "" {
		lines = append(lines, t)
	}
	if len(m.Stats) > 0 {
		lines = append(lines, "", "Uptime "+statLine(m.Stats))
	}
	if m.Link != "" {
		lines = append(lines, "", "Open in Hashi: "+m.Link)
	}
	return strings.Join(lines, "\n")
}

// Mail clients ignore style sheets unevenly, so every style is inline and the layout is tables.
var mailTemplate = template.Must(template.New("mail").Funcs(template.FuncMap{"percent": percent}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.M.Subject}}</title></head>
<body style="margin:0;padding:0;background:{{.P.Canvas}};">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{.P.Canvas}};"><tr><td align="center" style="padding:24px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<tr><td style="padding:0 4px 12px;color:{{.P.Muted}};font-size:13px;">Hashi</td></tr>
<tr><td style="background:{{.P.Surface}};border:1px solid {{.P.Border}};border-radius:12px;padding:24px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<tr><td style="color:{{.P.Text}};font-size:20px;line-height:28px;font-weight:600;"><span style="color:{{.Color}};">&#9679;</span> {{.M.Subject}}</td></tr>
{{if .M.Body}}<tr><td style="padding-top:8px;color:{{.P.Text}};font-size:15px;line-height:22px;">{{.M.Body}}</td></tr>{{end}}
{{if .When}}<tr><td style="padding-top:4px;color:{{.P.Muted}};font-size:13px;line-height:20px;">{{.When}}</td></tr>{{end}}
</table>
{{if .M.Stats}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:20px;border-top:1px solid {{.P.Border}};"><tr>
{{range .M.Stats}}<td style="padding-top:14px;"><div style="color:{{$.P.Muted}};font-size:12px;line-height:18px;">Uptime, {{.Label}}</div><div style="color:{{$.P.Text}};font-size:17px;line-height:24px;font-weight:600;">{{percent .Percent}}</div></td>{{end}}
</tr></table>{{end}}
{{if .M.Link}}<table role="presentation" cellpadding="0" cellspacing="0" style="margin-top:20px;"><tr><td style="background:{{.P.Accent}};border-radius:8px;"><a href="{{.M.Link}}" style="display:inline-block;padding:9px 16px;color:{{.P.AccentText}};font-size:14px;font-weight:600;text-decoration:none;">Open in Hashi</a></td></tr></table>{{end}}
</td></tr></table>
</td></tr></table>
</body></html>
`))

func mailHTML(m Message) (string, error) {
	p, ok := palettes[m.Theme]
	if !ok {
		p = palettes["graphite"]
	}
	color, ok := stateColors[m.State]
	if !ok {
		color = p.Muted
	}
	var b bytes.Buffer
	err := mailTemplate.Execute(&b, struct {
		M     Message
		P     palette
		Color string
		When  string
	}{m, p, color, timeLine(m)})
	if err != nil {
		return "", fmt.Errorf("render mail: %w", err)
	}
	return b.String(), nil
}
