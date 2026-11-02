package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The heartbeat needs one POST and one small flat answer. That small need is written out here because net/http and
// encoding/json are most of what this agent would otherwise keep in memory.

const maxAnswer = 16 << 10

type request struct{ Method, URL, Bearer, Body string }

type response struct {
	Status int
	Body   string
	// Root is the certificate at the top of the chain that verified the server, for https.
	Root []byte
}

// tlsVerifyError says the server's certificate did not check out, as opposed to the network failing.
type tlsVerifyError struct{ err error }

func (e *tlsVerifyError) Error() string { return e.err.Error() }

func roundTrip(ctx context.Context, r request, roots *x509.CertPool) (*response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, out, err := open(ctx, r, roots)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	raw := make([]byte, 0, 2048)
	buf := make([]byte, 2048)
	for len(raw) <= maxAnswer {
		n, err := conn.Read(buf)
		raw = append(raw, buf[:n]...)
		if err != nil {
			break // the server closes after the answer; a cut answer fails below
		}
	}
	out.Status, out.Body, err = parseHTTP(raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// open connects and sends the request; the caller reads the answer from the connection and closes it.
func open(ctx context.Context, r request, roots *x509.CertPool) (net.Conn, *response, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, nil, err
	}
	port := u.Port()
	var conn net.Conn
	out := &response{}
	switch u.Scheme {
	case "https":
		if port == "" {
			port = "443"
		}
		d := tls.Dialer{Config: &tls.Config{RootCAs: roots, ServerName: u.Hostname(), MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}}
		conn, err = d.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
		var bad *tls.CertificateVerificationError
		if errors.As(err, &bad) {
			return nil, nil, &tlsVerifyError{err}
		}
		if err == nil {
			if ch := conn.(*tls.Conn).ConnectionState().VerifiedChains; len(ch) > 0 { //nolint:errcheck // tls.Dialer returns *tls.Conn
				out.Root = ch[0][len(ch[0])-1].Raw
			}
		}
	case "http":
		if port == "" {
			port = "80"
		}
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	default:
		return nil, nil, fmt.Errorf("unsupported address %q", u.Scheme)
	}
	if err != nil {
		return nil, nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	var b strings.Builder
	b.WriteString(r.Method + " " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUser-Agent: hashi-pulse\r\nConnection: close\r\n")
	if r.Bearer != "" {
		b.WriteString("Authorization: Bearer " + r.Bearer + "\r\n")
	}
	if r.Method == "POST" {
		b.WriteString("Content-Type: application/json\r\nContent-Length: " + strconv.Itoa(len(r.Body)) + "\r\n")
	}
	b.WriteString("\r\n" + r.Body)
	if _, err := conn.Write([]byte(b.String())); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, out, nil
}

// parseHTTP reads a status and body, for the body forms a server or reverse proxy may use with Connection: close.
func parseHTTP(raw []byte) (int, string, error) {
	head, body, ok := bytes.Cut(raw, []byte("\r\n\r\n"))
	if !ok {
		return 0, "", errors.New("incomplete answer")
	}
	lines := strings.Split(string(head), "\r\n")
	f := strings.Fields(lines[0])
	if len(f) < 2 || !strings.HasPrefix(f[0], "HTTP/1.") {
		return 0, "", errors.New("not an HTTP answer")
	}
	status, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, "", errors.New("not an HTTP answer")
	}
	length, chunked := -1, false
	for _, l := range lines[1:] {
		k, v, _ := strings.Cut(l, ":")
		v = strings.TrimSpace(v)
		switch strings.ToLower(k) {
		case "content-length":
			if length, err = strconv.Atoi(v); err != nil {
				return 0, "", errors.New("bad answer length")
			}
		case "transfer-encoding":
			chunked = strings.EqualFold(v, "chunked")
		}
	}
	switch {
	case chunked:
		if body, err = unchunk(body); err != nil {
			return 0, "", err
		}
	case length >= 0:
		if len(body) < length {
			return 0, "", errors.New("cut off answer")
		}
		body = body[:length]
	}
	return status, string(body), nil
}

func unchunk(b []byte) ([]byte, error) {
	var out []byte
	for {
		line, rest, ok := bytes.Cut(b, []byte("\r\n"))
		if !ok {
			return nil, errors.New("cut off answer")
		}
		size, _, _ := bytes.Cut(line, []byte(";"))
		n, err := strconv.ParseUint(strings.TrimSpace(string(size)), 16, 16)
		if err != nil {
			return nil, errors.New("bad chunk in answer")
		}
		if n == 0 {
			return out, nil
		}
		if uint64(len(rest)) < n+2 {
			return nil, errors.New("cut off answer")
		}
		out = append(out, rest[:n]...)
		b = rest[n+2:]
	}
}

// quote writes a JSON string. Machine and interface names come from the machine, so everything is escaped properly.
func quote(b *strings.Builder, s string) {
	const hex = "0123456789abcdef"
	b.WriteByte('"')
	for _, r := range strings.ToValidUTF8(s, string(utf8.RuneError)) {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hex[r>>4])
			b.WriteByte(hex[r&0xf])
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

func (x beat) json() string {
	var b strings.Builder
	b.WriteString(`{"version":`)
	quote(&b, x.Version)
	b.WriteString(`,"hostname":`)
	quote(&b, x.Hostname)
	b.WriteString(`,"candidates":[`)
	for i, c := range x.Candidates {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"interface":`)
		quote(&b, c.Interface)
		b.WriteString(`,"address":`)
		quote(&b, c.Address)
		b.WriteByte('}')
	}
	b.WriteString(`]`)
	if x.Selected != "" {
		b.WriteString(`,"selected_interface":`)
		quote(&b, x.Selected)
	}
	b.WriteString(`,"interval_seconds":` + strconv.Itoa(x.Interval) + `,"sent_at":`)
	quote(&b, x.SentAt.Format(time.RFC3339Nano))
	if x.InDocker {
		b.WriteString(`,"in_docker":true`)
	}
	if x.ReportedPublic != "" {
		b.WriteString(`,"reported_public_ip":`)
		quote(&b, x.ReportedPublic)
	}
	b.WriteByte('}')
	return b.String()
}

// parseReply reads Hashi's flat answer. A field it does not recognize is passed over, so Hashi can add some; a missing
// field keeps its zero value, as a JSON decoder would; anything not an object is an error.
func parseReply(s string) (reply, error) {
	var r reply
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return r, errors.New("the answer is not an object")
	}
	r.CheckPublicIP = jsonValue(s, "check_public_ip") == "true"
	for key, dst := range map[string]*string{"version": &r.Version, "update_from": &r.UpdateFrom} {
		if v := jsonValue(s, key); v != "" {
			u, err := strconv.Unquote(v)
			if err != nil {
				return r, errors.New("bad " + key + " in the answer")
			}
			*dst = u
		}
	}
	if v := jsonValue(s, "interval_seconds"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return r, errors.New("bad interval in the answer")
		}
		r.IntervalSeconds = n
	}
	return r, nil
}

func jsonValue(s, key string) string {
	_, rest, ok := strings.Cut(s, `"`+key+`"`)
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t\r\n")
	if !strings.HasPrefix(rest, ":") {
		return ""
	}
	rest = strings.TrimLeft(rest[1:], " \t\r\n")
	if i := strings.IndexAny(rest, ",} \t\r\n"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// stateEnv carries what a beat must remember across the sleeper. It lives only as long as this process, never on disk.
const stateEnv = "HASHI_PULSE_STATE"

type state struct {
	Due      bool // the sleeper has waited, so beat right away
	Failures int
	Check    bool
	Checked  int64
	Reported string
	Interval int
	Root     []byte
	RootAt   int64
	// Tried is the version an update was last tried for: a failed one waits for a restart, not the next beat.
	Tried string
	// Restart: an update replaced this file, so an agent that stays in memory starts it.
	Restart bool
}

func (s state) encode() string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	return strings.Join([]string{b(s.Due), strconv.Itoa(s.Failures), b(s.Check), strconv.FormatInt(s.Checked, 10), s.Reported, strconv.Itoa(s.Interval), base64.RawStdEncoding.EncodeToString(s.Root), strconv.FormatInt(s.RootAt, 10), s.Tried}, ",")
}

// decodeState gives a fresh start for anything unreadable: the state only saves work.
func decodeState(v string) state {
	f := strings.Split(v, ",")
	if len(f) != 9 {
		return state{}
	}
	var s state
	var err [5]error
	s.Due, s.Check = f[0] == "1", f[2] == "1"
	s.Failures, err[0] = strconv.Atoi(f[1])
	s.Checked, err[1] = strconv.ParseInt(f[3], 10, 64)
	s.Reported = f[4]
	s.Interval, err[2] = strconv.Atoi(f[5])
	s.Root, err[3] = base64.RawStdEncoding.DecodeString(f[6])
	s.RootAt, err[4] = strconv.ParseInt(f[7], 10, 64)
	s.Tried = f[8]
	if errors.Join(err[:]...) != nil {
		return state{}
	}
	if len(s.Root) == 0 {
		s.Root = nil
	}
	return s
}

// pool is the one kept root as a trust list, or nil for the machine's full list when there is none or it is too old.
func (s *state) pool(now time.Time) *x509.CertPool {
	if s.Root == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(s.Root)
	if err != nil || now.Sub(time.Unix(s.RootAt, 0)) >= rootMaxAge {
		s.Root = nil
		return nil
	}
	p := x509.NewCertPool()
	p.AddCert(cert)
	return p
}
