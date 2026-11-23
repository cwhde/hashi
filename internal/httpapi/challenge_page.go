package httpapi

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"net/http"
	"net/netip"
	"net/url"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// The challenge page (06 §11) in the install's theme: the site, the visitor's address and the time in a side band,
// one line on why (the engine's last cause, never its limits), and the Cap widget styled through its CSS variables.

//go:embed challengefonts/*.woff2
var challengeFonts embed.FS

const challengeFontPath = services.ChallengePath + "/font/"

// challengeFontFiles are the page's two fonts by name, read once; a request picks one by name and nothing else.
var challengeFontFiles = func() map[string][]byte {
	m := map[string][]byte{}
	for _, n := range []string{"sans.woff2", "mono.woff2"} {
		b, err := challengeFonts.ReadFile("challengefonts/" + n)
		if err != nil {
			panic(err) // embedded at build time
		}
		m[n] = b
	}
	return m
}()

// fontURL is a font's address with its content hash, so browsers keep it until it changes (10c §8).
func fontURL(name string) string {
	h := sha256.Sum256(challengeFontFiles[name])
	return challengeFontPath + name + "?v=" + hex.EncodeToString(h[:6])
}

var challengeSans, challengeMono = fontURL("sans.woff2"), fontURL("mono.woff2")

// challengeFont serves the page's two fonts (Instrument Sans, Geist Mono; SIL OFL, licences next to the files) from
// the challenge name itself, so the page loads nothing from anywhere else.
func challengeFont(w http.ResponseWriter, r *http.Request) {
	b, ok := challengeFontFiles[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// challengeReasons say in one line why an address is challenged. Only the kind of cause shows, never a limit or count.
// A service that challenges every visitor, and a cause not listed here, show no line.
var challengeReasons = map[string]string{
	"l7_rate":   "This address sent many requests in a short time.",
	"l7_errors": "Many requests from this address failed.",
	"l7_waf":    "Some requests from this address looked like an attack.",
	"manual":    "The site owner asked for a check on this address.",
}

// challengeReason is the line for a visitor challenged by the engine (scope global); empty when unknown.
func (s *Server) challengeReason(ctx context.Context, scope string, ip netip.Addr) string {
	if scope != scopeGlobal || !ip.IsValid() {
		return ""
	}
	c, err := s.Store.R.GetAddressCounters(ctx, security.KeyString(ip.Unmap()))
	if err != nil {
		return ""
	}
	return challengeReasons[c.LastSource]
}

func svgURL(svg string) template.URL {
	return template.URL("data:image/svg+xml," + url.PathEscape(svg)) //nolint:gosec // built here from fixed markup and theme colours checked as #rrggbb
}

func cssURL(svg string) template.CSS {
	return template.CSS(`url("data:image/svg+xml,` + url.PathEscape(svg) + `")`) //nolint:gosec // as svgURL
}

type challengePageData struct {
	Nonce, Host, SiteKey, Verify, Scope, Service, RD, IP, Time, Why, Sans, Mono string
	P                                                                           settings.Palette
	Icon                                                                        template.URL
	Check, Cross                                                                template.CSS
}

func (s *Server) challengePageData(ctx context.Context, c settings.Captcha, nonce, host, scope, service, rd string, ip netip.Addr) challengePageData {
	a, _ := settings.Get[settings.Appearance](ctx, s.Store.R) // an unreadable setting falls back to the default theme
	p := settings.PaletteOf(a.Theme, a.CustomTheme)
	d := challengePageData{Nonce: nonce, Host: host, SiteKey: c.SiteKey, Verify: challengeVerifyPath, Scope: scope,
		Service: service, RD: rd, Time: s.now().UTC().Format("2006-01-02 15:04") + " UTC",
		Why: s.challengeReason(ctx, scope, ip), Sans: challengeSans, Mono: challengeMono, P: p}
	if ip.IsValid() {
		d.IP = ip.Unmap().String()
	}
	d.Icon = svgURL(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="14" fill="` +
		p.Accent + `"/><text x="32" y="45.5" text-anchor="middle" font-size="40" font-weight="600" fill="` + p.AccentText +
		`" font-family="Hiragino Sans,Noto Sans JP,sans-serif">橋</text></svg>`)
	// Cap's own check mark and error mark, in the accent instead of its green and red.
	d.Check = cssURL(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><style>@keyframes anim{0%{stroke-dashoffset:23.21px}to{stroke-dashoffset:0}}</style><path fill="none" stroke="` +
		p.Accent + `" stroke-linecap="round" stroke-linejoin="round" stroke-width="2.4" d="m5 12 5 5L20 7" style="stroke-dashoffset:0;stroke-dasharray:23.21px;animation:anim .5s ease"/></svg>`)
	d.Cross = cssURL(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 24 24"><g fill="none" stroke="` +
		p.Accent + `" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5M12 16.5v.01"/></g></svg>`)
	return d
}

var challengePage = template.Must(template.New("challenge").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex"><title>Security check</title><link rel="icon" href="{{.Icon}}">
<style nonce="{{.Nonce}}">
@font-face{font-family:"Instrument Sans";src:url({{.Sans}}) format("woff2");font-weight:400 700;font-display:swap}
@font-face{font-family:"Geist Mono";src:url({{.Mono}}) format("woff2");font-weight:400 600;font-display:swap}
:root{--canvas:{{.P.Canvas}};--shell:{{.P.Shell}};--surface-2:{{.P.Surface2}};--border:{{.P.Border}};--text:{{.P.Text}};
--muted:{{.P.Muted}};--accent:{{.P.Accent}};color-scheme:{{if .P.Dark}}dark{{else}}light{{end}}}
*{box-sizing:border-box}html,body{margin:0;height:100%}
body{background:var(--canvas);color:var(--text);font:15px/1.55 "Instrument Sans",system-ui,sans-serif;-webkit-font-smoothing:antialiased}
.mono{font-family:"Geist Mono",ui-monospace,monospace;font-size:.86em}
h1{font-weight:600;letter-spacing:-.01em;margin:0;font-size:clamp(1.6rem,3.4vw,2.4rem);line-height:1.15}
p{margin:0}
.page{min-height:100%;display:grid;grid-template-columns:minmax(0,5fr) minmax(0,7fr)}
.side{background:var(--shell);border-right:1px solid var(--border);padding:40px;display:flex;flex-direction:column;justify-content:space-between;gap:24px;overflow:hidden;position:relative}
.glyph{font:600 min(34vw,420px)/.85 "Hiragino Sans","Noto Sans JP",sans-serif;color:var(--accent);opacity:.14;position:absolute;right:-.12em;bottom:-.08em;pointer-events:none}
.host{font-size:clamp(1.4rem,3vw,2.1rem);font-weight:600;line-height:1.15;overflow-wrap:anywhere;position:relative}
.host span{display:block;font-size:.9rem;font-weight:400;color:var(--muted);margin-bottom:6px}
dl{margin:0;display:grid;gap:10px;font-size:.84rem;position:relative}dt{color:var(--muted)}dd{margin:0}
main{display:grid;align-content:center;padding:48px clamp(20px,6vw,80px);gap:18px;max-width:620px}
.why{border-left:3px solid var(--accent);padding:2px 0 2px 16px}
.note{color:var(--muted)}
#msg{height:1.4em;line-height:1.4;font-size:.9rem;color:var(--muted)}#msg.bad{color:var(--text);font-weight:500}
cap-widget{display:block;width:100%;
--cap-background:var(--surface-2);--cap-border-color:var(--border);--cap-border-radius:10px;--cap-color:var(--text);
--cap-checkbox-background:var(--canvas);--cap-checkbox-border:1.5px solid var(--muted);--cap-checkbox-border-radius:6px;
--cap-checkbox-size:24px;--cap-spinner-color:var(--accent);--cap-spinner-background-color:var(--border);--cap-spinner-thickness:3px;
--cap-font:"Instrument Sans",system-ui,sans-serif;--cap-focus-ring:var(--accent);--cap-troubleshoot-color:var(--muted);
--cap-widget-width:100%;--cap-widget-height:58px;--cap-widget-padding:14px 16px;--cap-gap:14px;
--cap-checkmark:{{.Check}};--cap-error-cross:{{.Cross}}}
@media (max-width:760px){.page{grid-template-columns:1fr;grid-template-rows:auto 1fr}
.side{border-right:0;border-bottom:1px solid var(--border);padding:24px 20px;gap:20px}
dl{display:flex;flex-wrap:wrap;gap:10px 24px}.glyph{font-size:180px}main{padding:28px 20px 40px;align-content:start}}
</style></head>
<body><div class="page">
<aside class="side"><span class="glyph" aria-hidden="true">橋</span>
<p class="host"><span>Requested site</span>{{.Host}}</p>
<dl>{{if .IP}}<div><dt>Address</dt><dd class="mono">{{.IP}}</dd></div>{{end}}<div><dt>Time</dt><dd class="mono">{{.Time}}</dd></div></dl>
</aside>
<main>
<h1>Security check</h1>
{{if .Why}}<p class="why">{{.Why}}</p>{{end}}
<p class="note">No personal data is collected.</p>
<cap-widget data-cap-api-endpoint="/{{.SiteKey}}/" data-cap-i18n-initial-state="Click to continue"
 data-cap-i18n-verify-aria-label="Click to continue" data-cap-i18n-verifying-label="Checking…"
 data-cap-i18n-verifying-aria-label="Checking…" data-cap-i18n-solved-label="Passed"
 data-cap-i18n-verified-aria-label="Passed, continuing to {{.Host}}" data-cap-i18n-error-label="Failed. Click to try again."
 data-cap-i18n-error-aria-label="Failed. Click to try again." data-cap-i18n-group-aria-label="Security check"
 data-cap-i18n-troubleshooting-label="Help with this check"></cap-widget>
<noscript><p>This check needs JavaScript. Turn it on for this site and reload the page.</p></noscript>
<p id="msg" role="status"></p>
</main></div>
<script nonce="{{.Nonce}}">
window.CAP_CUSTOM_WASM_URL = "/assets/cap_wasm_bg.wasm";
window.CAP_CUSTOM_HASHWX_URL = "/assets/hashwx.wasm";
window.CAP_SCRIPT_NONCE = window.CAP_CSS_NONCE = "{{.Nonce}}";
const widget = document.querySelector("cap-widget"), msg = document.getElementById("msg");
const say = (text, bad) => { msg.textContent = text; msg.className = bad ? "bad" : ""; };
// The widget says itself that it failed and can be clicked again.
widget.addEventListener("error", (e) => { say(""); console.warn("cap", e.detail); });
widget.addEventListener("solve", async (e) => {
  say("Continuing to {{.Host}}…");
  try {
    const r = await fetch("{{.Verify}}", {method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({token: e.detail.token, scope: {{.Scope}}, service: {{.Service}}, rd: {{.RD}}})});
    const j = await r.json();
    if (r.ok && j.redirect) { location.replace(j.redirect); return; }
    fail(r.status === 503 ? unavailable : "The check failed. Click to try again.");
  } catch { fail(unavailable); }
});
// Cap shows "Passed" before Hashi has answered; a refused check puts the widget back so it can be tried again.
const unavailable = "Checking is unavailable right now. Try again in a few minutes.";
function fail(text) { widget.reset(); say(text, true); }
</script>
<script nonce="{{.Nonce}}" src="/assets/widget.js"></script>
</body></html>
`))
