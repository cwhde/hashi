package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// Cookie names (02 §3.1). The bootstrap cookie exists only for bootstrap sessions over plain HTTP from a private
// network (02 §1.1 point 5).
const (
	cookieSession   = "__Host-hashi.session"
	cookieBootstrap = "hashi.bootstrap"
	headerCSRF      = "X-Hashi-CSRF"
)

type ctxKey struct{}

// request is what the HTTP layer knows about a call before the handler runs.
type request struct {
	ip      netip.Addr // invalid when unresolvable (fail closed)
	tls     bool
	ua      string
	method  string
	session *auth.Session
}

func info(ctx context.Context) *request {
	if r, ok := ctx.Value(ctxKey{}).(*request); ok {
		return r
	}
	return &request{}
}

func (r *request) ipString() string {
	if r.ip.IsValid() {
		return r.ip.String()
	}
	return ""
}

// withRequest resolves the client IP and the session cookie for every API call.
func (s *Server) withRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ri := &request{tls: s.IP.IsTLS(r), ua: r.UserAgent(), method: r.Method}
		if ip, err := s.IP.Resolve(r); err == nil {
			ri.ip = ip
		}
		if c, name := sessionCookie(r); c != "" {
			sess, err := s.Sessions.Validate(r.Context(), c, ri.ip)
			switch {
			case err == nil && name == cookieBootstrap && sess.Method != auth.MethodBootstrap:
				// The non-Secure cookie name may only ever carry a bootstrap session.
				clearCookie(w, name)
			case err == nil:
				ri.session = sess
			case errors.Is(err, auth.ErrNoSession):
				clearCookie(w, name)
			default:
				s.writeError(w, r, err)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, ri)))
	})
}

func sessionCookie(r *http.Request) (value, name string) {
	for _, n := range []string{cookieSession, cookieBootstrap} {
		if c, err := r.Cookie(n); err == nil && c.Value != "" {
			return c.Value, n
		}
	}
	return "", ""
}

func newCookie(name, value string, maxAge int) *http.Cookie {
	// Secure is off only for the bootstrap cookie over plain HTTP from a private network (02 §1.1 point 5).
	return &http.Cookie{ //nolint:gosec // see above; HttpOnly and SameSite=Strict are always set
		Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true,
		Secure: name == cookieSession, SameSite: http.SameSiteStrictMode,
	}
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, newCookie(name, "", -1))
}

// issueCookie returns the Set-Cookie value for a fresh session. Plain HTTP is allowed only for bootstrap sessions.
func issueCookie(ri *request, token string, sess *auth.Session) string {
	name := cookieSession
	if sess.Method == auth.MethodBootstrap && !ri.tls {
		name = cookieBootstrap
	}
	return newCookie(name, token, int(time.Until(sess.AbsoluteAt).Seconds())).String()
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// requireScopes enforces the generated x-hashi-* table: session, CSRF on unsafe methods, every scope, recent reauth.
func (s *Server) requireScopes(f gen.StrictHandlerFunc, op string) gen.StrictHandlerFunc {
	req, known := gen.Requirements[op]
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, body any) (any, error) {
		if !known {
			return nil, forbidden("operation " + op + " declares no scopes") // unreachable: scopegen fails first
		}
		if !req.NeedsSession() {
			return f(ctx, w, r, body)
		}
		ri := info(ctx)
		sess := ri.session
		if sess == nil {
			return nil, unauthenticated()
		}
		if unsafeMethod(r.Method) && !s.Sessions.CheckCSRF(sess, r.Header.Get(headerCSRF)) {
			s.deny(ctx, ri, op, "csrf")
			return nil, forbidden("Missing or wrong " + headerCSRF + " header; reload the page.")
		}
		scopes := make([]auth.Scope, len(req.Scopes))
		for i, sc := range req.Scopes {
			scopes[i] = auth.Scope(sc)
		}
		if !sess.Has(scopes...) {
			s.deny(ctx, ri, op, "scope")
			return nil, forbidden("This session is not allowed to do that.")
		}
		if req.Reauth && !sess.ReauthValid(s.now()) {
			return nil, reauthRequired()
		}
		return f(ctx, w, r, body)
	}
}

// deny audits a scope or CSRF failure (02 §3.5).
func (s *Server) deny(ctx context.Context, ri *request, op, why string) {
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{
		Actor: s.Audit.SessionActor(ri.session.IDHash), Action: "auth.denied", Target: op, Outcome: audit.Denied,
		IP: ri.ipString(), Detail: map[string]any{"reason": why},
	}); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
}

// actor is the audit actor for the current request.
func (s *Server) actor(ri *request) string {
	if ri.session == nil {
		return audit.Anonymous
	}
	return s.Audit.SessionActor(ri.session.IDHash)
}
