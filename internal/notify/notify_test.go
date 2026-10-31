package notify

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

type fake struct {
	sent []Message
	err  error
}

func (f *fake) Send(_ context.Context, m Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func fixture(t *testing.T) (*Service, *fake, *time.Time) {
	t.Helper()
	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	s := New(st, v)
	s.build = func(db.NotificationProvider, secrets.Value) (Sender, error) { return f, nil }
	now := time.Now()
	s.now = func() time.Time { return now }
	return s, f, &now
}

func TestRoutingCooldownAndRecovery(t *testing.T) {
	s, f, now := fixture(t)
	ctx := t.Context()
	p, err := s.CreateSMTP(ctx, "Mail", SMTPConfig{Host: "mail.example.com", Port: 587, TLS: SMTPStartTLS, From: "a@example.com", To: "b@example.com"}, secrets.New("pw"))
	if err != nil {
		t.Fatal(err)
	}
	e := db.MonitorEndpoint{ID: "e1", Name: "Wiki"}
	if err := s.st.Q.InsertMonitorEndpoint(ctx, db.InsertMonitorEndpointParams{ID: "e1", SourceType: "user", Name: "Wiki", GroupName: "G", IntervalSeconds: 60, TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	change := func(from, to string) {
		t.Helper()
		if err := s.StateChanged(ctx, s.st.Q, e, from, to, "why"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
	}
	change("up", "down")
	if len(f.sent) != 0 {
		t.Fatal("no route, no message")
	}
	if err := s.st.Q.PutNotificationRoute(ctx, db.PutNotificationRouteParams{ProviderID: nullStr(p.ID), MinSeverity: "down", CooldownSeconds: 600, Recovery: 1}); err != nil {
		t.Fatal(err)
	}
	change("unknown", "up")
	change("up", "degraded") // below the severity threshold
	if len(f.sent) != 0 {
		t.Fatalf("%+v", f.sent)
	}
	setState := func(st string) {
		t.Helper()
		if err := s.st.Q.SetMonitorEndpointResult(ctx, db.SetMonitorEndpointResultParams{State: st, ID: "e1"}); err != nil {
			t.Fatal(err)
		}
	}
	setState("down")
	change("up", "down")
	if len(f.sent) != 1 || f.sent[0].Subject != "Wiki is down" {
		t.Fatalf("alert %+v", f.sent)
	}
	setState("up")
	change("down", "up")
	if len(f.sent) != 2 || f.sent[1].Subject != "Wiki is up again" {
		t.Fatalf("recovery %+v", f.sent)
	}
	// Review of S7: down again inside the cooldown and staying down: the alert waits, then goes.
	*now = now.Add(5 * time.Minute)
	setState("down")
	change("up", "down")
	if len(f.sent) != 2 {
		t.Fatalf("cooldown %+v", f.sent)
	}
	*now = now.Add(6 * time.Minute)
	if _, err := s.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 3 || f.sent[2].Subject != "Wiki is down" {
		t.Fatalf("outage left at up again: %+v", f.sent)
	}
	// Recovered while an alert waited: no alert, and no recovery for an alarm nobody got.
	setState("up")
	change("down", "up")
	if len(f.sent) != 4 {
		t.Fatalf("recovery %+v", f.sent)
	}
	*now = now.Add(time.Minute)
	setState("down")
	change("up", "down")
	setState("up")
	change("down", "up")
	*now = now.Add(time.Hour)
	if _, err := s.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 4 {
		t.Fatalf("a blip inside the cooldown sent %+v", f.sent)
	}
}

func TestFailedDeliveryRetriesThenStops(t *testing.T) {
	s, f, now := fixture(t)
	ctx := t.Context()
	p, err := s.CreateSMTP(ctx, "Mail", SMTPConfig{Host: "mail.example.com", Port: 465, TLS: SMTPTLS, From: "a@example.com", To: "b@example.com"}, secrets.Value{})
	if err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("server said no")
	if err := s.EnqueueTest(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	for range MaxAttempts {
		*now = now.Add(24 * time.Hour)
		if _, err := s.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := s.st.R.ListNotificationDeliveries(ctx, 5)
	if len(rows) != 1 || rows[0].Status != "failed" || rows[0].LastError != "server said no" {
		t.Fatalf("%+v", rows)
	}
}

func TestSMTPValidateRefusesPlainText(t *testing.T) {
	c := SMTPConfig{Host: "h", Port: 25, TLS: "none", From: "a@example.com", To: "b@example.com"}
	if err := c.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestMailHeadersCannotBeInjected(t *testing.T) {
	raw, err := buildMail(SMTPConfig{From: "a@example.com", To: "b@example.com"}, Message{Subject: "x\r\nBcc: evil@example.com", Body: "b"}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	b := string(raw)
	head, _, _ := strings.Cut(b, "\r\n\r\n")
	for _, l := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(l, "Bcc:") {
			t.Fatalf("injected header in\n%s", b)
		}
	}
}

// Replays the captured Bot API answers (testdata/contracts/telegram): chat discovery, sending, both errors. The token
// stays out of every error, even a transport error whose URL holds it.
func TestTelegramAgainstContract(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("../../testdata/contracts/telegram/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "bad"):
			w.WriteHeader(401)
			_, _ = w.Write(read("getMe-bad-token.json"))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write(read("getUpdates.json"))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["chat_id"] == "1" {
				w.WriteHeader(400)
				_, _ = w.Write(read("sendMessage-bad-chat.json"))
				return
			}
			_, _ = w.Write(read("sendMessage.json"))
		}
	}))
	defer srv.Close()
	ctx := t.Context()
	bot := Telegram{Token: secrets.New("123:good"), ChatID: "100000001", Base: srv.URL}
	chats, err := bot.DiscoverChats(ctx)
	if err != nil || len(chats) != 1 || chats[0].ID != "100000001" || chats[0].Title != "Example" {
		t.Fatalf("chats %+v %v", chats, err)
	}
	if err := bot.Send(ctx, Message{Subject: "Wiki is down", Body: "why"}); err != nil {
		t.Fatal(err)
	}
	if body["text"] != "<b>Wiki is down</b>\nwhy" || body["parse_mode"] != "HTML" {
		t.Fatalf("text %v", body["text"])
	}
	bot.ChatID = "1"
	if err := bot.Send(ctx, Message{Subject: "x"}); err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("bad chat %v", err)
	}
	if _, err := (Telegram{Token: secrets.New("bad"), Base: srv.URL}).DiscoverChats(ctx); err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("bad token %v", err)
	}
	srv.Close()
	err = (Telegram{Token: secrets.New("123:secret-token"), ChatID: "1", Base: srv.URL}).Send(ctx, Message{Subject: "x"})
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport error %v", err)
	}
	if err := (TelegramConfig{ChatID: "nope"}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

// Against a local SMTP server over implicit TLS: login, envelope and body arrive; a refused login is an error.
func TestSMTPSendsOverTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	cert := srv.TLS.Certificates[0]
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	srv.Close()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var mu sync.Mutex
	var got []string
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			r := bufio.NewReader(c)
			_, _ = c.Write([]byte("220 test\r\n"))
			data := false
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					break
				}
				mu.Lock()
				got = append(got, strings.TrimSpace(l))
				mu.Unlock()
				switch {
				case data:
					if l == ".\r\n" {
						data = false
						_, _ = c.Write([]byte("250 OK\r\n"))
					}
				case strings.HasPrefix(l, "EHLO"):
					_, _ = c.Write([]byte("250-test\r\n250 AUTH PLAIN\r\n"))
				case strings.HasPrefix(l, "AUTH"):
					if strings.Contains(l, base64.StdEncoding.EncodeToString([]byte("\x00user\x00wrong"))) {
						_, _ = c.Write([]byte("535 no\r\n"))
					} else {
						_, _ = c.Write([]byte("235 ok\r\n"))
					}
				case strings.HasPrefix(l, "DATA"):
					data = true
					_, _ = c.Write([]byte("354 go\r\n"))
				case strings.HasPrefix(l, "QUIT"):
					_, _ = c.Write([]byte("221 bye\r\n"))
				default:
					_, _ = c.Write([]byte("250 OK\r\n"))
				}
			}
			_ = c.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	cfg := SMTPConfig{Host: "127.0.0.1", Port: addr.Port, TLS: SMTPTLS, Username: "user", From: "status@example.com", To: "me@example.com"}
	client := &tls.Config{RootCAs: pool, ServerName: "example.com"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := (SMTP{Config: cfg, Password: secrets.New("right"), RootsConfig: client}).Send(ctx, Message{Subject: "Wiki is down", Body: "why"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	all := strings.Join(got, "\n")
	mu.Unlock()
	for _, want := range []string{"MAIL FROM:<status@example.com>", "RCPT TO:<me@example.com>", "Subject: Wiki is down", "why"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if err := (SMTP{Config: cfg, Password: secrets.New("wrong"), RootsConfig: client}).Send(ctx, Message{Subject: "x"}); err == nil {
		t.Fatal("refused login passed")
	}
}

// The mail's colours are the app's themes.
func TestMailPalettesMatchThemes(t *testing.T) {
	css, err := os.ReadFile("../../web/src/routes/layout.css")
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range palettes {
		_, rest, ok := strings.Cut(string(css), "[data-theme='"+name+"'] {")
		if !ok {
			t.Fatalf("theme %s is not in layout.css", name)
		}
		block, _, _ := strings.Cut(rest, "}")
		for token, want := range map[string]string{"canvas": p.Canvas, "surface": p.Surface, "border": p.Border, "text": p.Text, "muted": p.Muted, "accent": p.Accent, "accent-text": p.AccentText} {
			if !strings.Contains(block, "--h-"+token+": "+want+";") {
				t.Errorf("%s: --h-%s is not %s", name, token, want)
			}
		}
	}
}

// A message carries no raw check error: it would hand internal addresses to the mail or chat service.
func TestAlertTextHidesInternalDetails(t *testing.T) {
	for raw, want := range map[string]string{
		`http: Get "http://10.0.0.5:8080/": dial tcp 10.0.0.5:8080: connect: connection refused`: "The service refused the connection.",
		`tcp: dial tcp 10.0.0.5:22: i/o timeout`:                                                 "The service did not answer in time.",
		"http: HTTP 503, expected 200-399":                                                       "The service answered with an error.",
		"something new at 10.0.0.5":                                                              "Its check failed.",
	} {
		if got := plainReason(raw); got != want {
			t.Errorf("%q: %q", raw, got)
		}
	}
}

// Both parts survive, service names are escaped, and the chosen theme reaches the page.
func TestStyledMessages(t *testing.T) {
	m := Message{Subject: "<Wiki> is down", Body: "The service refused the connection.", State: "down", Theme: "verdant", Link: "https://admin.example.com/status/e1",
		At: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), Stats: []Stat{{"30 d", 99.5}}}
	raw, err := buildMail(SMTPConfig{From: "a@example.com", To: "b@example.com"}, m, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(p) // the reader undoes quoted-printable
		if err != nil {
			t.Fatal(err)
		}
		parts[strings.Split(p.Header.Get("Content-Type"), ";")[0]] = string(b)
	}
	if !strings.Contains(parts["text/plain"], "Uptime 30 d: 99.5 %") {
		t.Errorf("text part:\n%s", parts["text/plain"])
	}
	page := parts["text/html"]
	for _, want := range []string{"&lt;Wiki&gt; is down", "#0a1a12", "#EF4444", "99.5 %", `href="https://admin.example.com/status/e1"`} {
		if !strings.Contains(page, want) {
			t.Errorf("html lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "ZgotmplZ") {
		t.Errorf("template refused a value:\n%s", page)
	}
	tg := telegramHTML(m)
	if !strings.Contains(tg, "🔴 <b>&lt;Wiki&gt; is down</b>") || !strings.Contains(tg, `<a href="https://admin.example.com/status/e1">`) {
		t.Errorf("telegram: %s", tg)
	}
}

func TestUpdateKeepsSecretUnlessGiven(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := t.Context()
	p, err := s.CreateTelegram(ctx, "Bot", TelegramConfig{ChatID: "1"}, secrets.New("token"))
	if err != nil {
		t.Fatal(err)
	}
	keep := func(string) (any, error) { return TelegramConfig{ChatID: "2"}, nil }
	r, err := s.Update(ctx, p.ID, "Renamed", keep, secrets.Value{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Renamed" || r.SecretID != p.SecretID || !strings.Contains(r.ConfigJson, `"2"`) {
		t.Fatalf("%+v", r)
	}
	r2, err := s.Update(ctx, p.ID, "Renamed", keep, secrets.New("new"))
	if err != nil {
		t.Fatal(err)
	}
	if r2.SecretID == p.SecretID {
		t.Fatal("a new secret must replace the old one")
	}
	if _, err := s.st.R.GetSecret(ctx, p.SecretID.String); err == nil {
		t.Fatal("the old secret must be gone")
	}
	if _, err := s.Update(ctx, "nope", "x", keep, secrets.Value{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}
