package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// TypeSMTP is the SMTP provider.
const TypeSMTP = "smtp"

// TLS modes (07 §6).
const (
	SMTPStartTLS = "starttls"
	SMTPTLS      = "tls"
)

// SMTPConfig is everything but the password.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	TLS      string `json:"tls"`
	Username string `json:"username,omitempty"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// Validate refuses a config that cannot send. There is no unencrypted mode.
func (c SMTPConfig) Validate() error {
	if c.Host == "" || c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("%w: a host and port are required", ErrInvalid)
	}
	if c.TLS != SMTPStartTLS && c.TLS != SMTPTLS {
		return fmt.Errorf("%w: TLS must be starttls or tls", ErrInvalid)
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return fmt.Errorf("%w: the from address is not valid", ErrInvalid)
	}
	if _, err := mail.ParseAddress(c.To); err != nil {
		return fmt.Errorf("%w: the to address is not valid", ErrInvalid)
	}
	return nil
}

// SMTP sends through a mail server with the standard library (invariant 18).
type SMTP struct {
	Config   SMTPConfig
	Password secrets.Value
	// RootsConfig overrides the TLS config (tests against a local server).
	RootsConfig *tls.Config
}

// Send delivers m. Header values are sanitized: subjects hold service names the admin typed.
func (s SMTP) Send(ctx context.Context, m Message) error {
	c := s.Config
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	cfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if s.RootsConfig != nil {
		cfg = s.RootsConfig
	}
	var conn net.Conn
	var err error
	if c.TLS == SMTPTLS {
		conn, err = (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return fmt.Errorf("greeting: %w", err)
	}
	defer func() { _ = cl.Close() }()
	if c.TLS == SMTPStartTLS {
		if err := cl.StartTLS(cfg); err != nil { // fails when the server does not offer it: never falls back to plain text
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, s.Password.Reveal(), c.Host)); err != nil {
			return fmt.Errorf("sign in: %w", err)
		}
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return err
	}
	to, err := mail.ParseAddress(c.To)
	if err != nil {
		return err
	}
	if err := cl.Mail(from.Address); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if err := cl.Rcpt(to.Address); err != nil {
		return fmt.Errorf("to: %w", err)
	}
	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	raw, err := buildMail(c, m, time.Now())
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("data: %w", err)
	}
	return cl.Quit()
}

// buildMail makes a mail with a plain text part and a styled one; the client shows the best it can.
func buildMail(c SMTPConfig, m Message, now time.Time) ([]byte, error) {
	page, err := mailHTML(m)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ typ, content string }{{"text/plain", mailText(m)}, {"text/html", page}} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.typ + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		clean.Replace(c.From), clean.Replace(c.To), mime.QEncoding.Encode("utf-8", clean.Replace(stateEmojiPrefix(m)+m.Subject)), now.Format(time.RFC1123Z), mw.Boundary())
	b.Write(body.Bytes())
	return b.Bytes(), nil
}
