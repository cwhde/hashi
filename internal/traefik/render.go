package traefik

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"

	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
)

// Fixed host paths (04 §2 step 3, 04 §3). Only the binary and the config directory are overridable per connection.
const (
	LogDir   = "/var/log/hashi/traefik"
	StateDir = "/var/lib/hashi/traefik"

	DefaultBinary    = "/usr/local/bin/traefik"
	DefaultConfigDir = "/etc/hashi/traefik"
)

// Paths are a connection's paths on the host.
type Paths struct{ Binary, ConfigDir string }

func (p Paths) Static() string  { return p.ConfigDir + "/traefik.yaml" }
func (p Paths) Dynamic() string { return p.ConfigDir + "/dynamic" }
func (p Paths) Certs() string   { return p.ConfigDir + "/certs" }

// safePath allows what the defaults look like: absolute, no spaces, quotes or shell characters, because paths end up
// in the service definition and in commands.
var safePath = regexp.MustCompile(`^(/[A-Za-z0-9._-]+)+$`)

// Validate refuses paths that are not plain absolute paths.
func (p Paths) Validate() error {
	for _, s := range []string{p.Binary, p.ConfigDir} {
		if !safePath.MatchString(s) {
			return fmt.Errorf("path %q must be absolute and contain only letters, digits, '.', '_', '-' and '/'", s)
		}
	}
	return nil
}

// Directory is one directory Hashi creates on a host.
type Directory struct {
	Path string
	Mode uint32
}

// Directories are Hashi's directories on a host, parents first (04 §2 step 3).
func (p Paths) Directories() []Directory {
	return []Directory{
		{p.ConfigDir, 0o755}, {p.Dynamic(), 0o755}, {p.Certs(), 0o700},
		{LogDir, 0o755}, {StateDir, 0o700},
	}
}

// File kinds, in the order Apply writes them: certificates before the YAML that lists them (03 §5.3), dynamic files
// before the static file (whose change restarts Traefik).
const (
	KindCert    = "cert"
	KindDynamic = "dynamic"
	KindSystem  = "system" // outside Traefik's config (logrotate); written with the dynamic files, no restart
	KindStatic  = "static"
)

// File is one rendered file. Secret files (certificate keys) never appear in plans, logs or API responses: only their hash.
type File struct {
	Path   string
	Kind   string
	Mode   uint32
	Data   []byte
	Secret bool
}

// Hash is the hex SHA-256 of the content, the value compared with the host and stored as last_applied_hash.
func (f File) Hash() string {
	h := sha256.Sum256(f.Data)
	return hex.EncodeToString(h[:])
}

// Cert is a Hashi-issued certificate to distribute.
type Cert struct {
	Name    string // file base name: the certificate's name (its root domain)
	CertPEM []byte
	KeyPEM  secrets.Value
}

// Options are the inputs of one host's render.
type Options struct {
	Paths Paths
	// APIPort serves the API (Verify) and ping on 127.0.0.1; ForwardAuthPort is the loopback hashi-fa entry point.
	APIPort, ForwardAuthPort int
	// HashiURL is Hashi's internal URL (setup step 2) that edges reach forward auth on, e.g. http://192.0.2.5:3340.
	HashiURL string
	// TrustedIPs are the firewall hosts' internal addresses whose X-Forwarded-* headers websecure trusts (04 §3; S6).
	TrustedIPs []string
	Certs      []Cert
	// Services are every stored service; Render keeps the renderable ones (04 §7).
	Services []services.Service
	// EdgeToken is set on requests the edge sends to Hashi (01 §8); empty renders no edge middleware.
	EdgeToken secrets.Value
}

// Router, middleware and service names other code (Verify, S5 services) refers to.
const (
	MiddlewareHeaders    = "hashi-headers"
	MiddlewareCompress   = "hashi-compress"
	MiddlewareAuthOpen   = "hashi-auth-open"
	MiddlewareAuthClosed = "hashi-auth-closed"
	RouterForwardAuth    = "hashi-fa"
	ServiceFailover      = "hashi-fa-failover"
	ServiceHashi         = "hashi-fa-primary"
	RouterHealthOpen     = "hashi-health-open"
	RouterHealthClosed   = "hashi-health-closed"

	// HealthPath is the edge path the health routers answer on, for every host name: /open goes through fail-open
	// forward auth, /closed through fail-closed (06 §2.1). ping@internal answers 200.
	HealthPath = "/.well-known/hashi/health"

	forwardAuthPath = "/api/edge-auth/forward"
	readyPath       = "/readyz"
)

var certName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

func (o Options) validate() error {
	if err := o.Paths.Validate(); err != nil {
		return err
	}
	for _, port := range []int{o.APIPort, o.ForwardAuthPort} {
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of range", port)
		}
	}
	if o.APIPort == o.ForwardAuthPort {
		return errors.New("API port and forward-auth port must differ")
	}
	u, err := url.Parse(o.HashiURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.User != nil {
		return fmt.Errorf("internal Hashi URL %q must be http(s)://host[:port]", o.HashiURL)
	}
	seen := map[string]bool{}
	for _, c := range o.Certs {
		if !certName.MatchString(c.Name) || seen[c.Name] {
			return fmt.Errorf("certificate name %q is invalid or repeated", c.Name)
		}
		seen[c.Name] = true
		if len(c.CertPEM) == 0 || c.KeyPEM.IsZero() {
			return fmt.Errorf("certificate %q has no certificate or key", c.Name)
		}
	}
	return nil
}

// Render produces every file Hashi owns on a host, in apply order. The same options always give the same bytes, so an
// unchanged host gets no writes (invariant 8).
func Render(o Options) ([]File, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	hashi := o.HashiURL
	if hashi[len(hashi)-1] == '/' {
		hashi = hashi[:len(hashi)-1]
	}
	certs := append([]Cert(nil), o.Certs...)
	sort.Slice(certs, func(i, j int) bool { return certs[i].Name < certs[j].Name })

	var files []File
	var tlsCerts []certFile
	var sums bytes.Buffer
	for _, c := range certs {
		crt, key := o.Paths.Certs()+"/"+c.Name+".crt", o.Paths.Certs()+"/"+c.Name+".key"
		files = append(files,
			File{Path: crt, Kind: KindCert, Mode: 0o644, Data: c.CertPEM},
			File{Path: key, Kind: KindCert, Mode: 0o600, Data: []byte(c.KeyPEM.Reveal()), Secret: true})
		tlsCerts = append(tlsCerts, certFile{CertFile: crt, KeyFile: key})
		h := sha256.Sum256(c.CertPEM)
		fmt.Fprintf(&sums, "# %s.crt sha256 %s\n", c.Name, hex.EncodeToString(h[:]))
	}

	dyn := func(name string, v any, head string) error {
		b, err := marshal(v)
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		files = append(files, File{Path: o.Paths.Dynamic() + "/" + name, Kind: KindDynamic, Mode: 0o644,
			Data: append([]byte(generated+head), b...)})
		return nil
	}

	// hashi-certs.yaml carries every certificate's hash, so it changes whenever a certificate does; Traefik v3.7 reloads
	// certificate files only then (testdata/contracts/traefik/README.md finding 1).
	if err := dyn("hashi-certs.yaml", dynamicConfig{TLS: &tlsConfig{Certificates: tlsCerts}}, sums.String()); err != nil {
		return nil, err
	}
	if err := dyn("00-hashi-core.yml", dynamicConfig{HTTP: &httpConfig{Middlewares: map[string]middleware{
		MiddlewareHeaders: {Headers: &headers{
			STSSeconds: 31536000, STSIncludeSubdomains: true, ContentTypeNosniff: true,
			ReferrerPolicy: "strict-origin-when-cross-origin",
		}},
		MiddlewareCompress: {Compress: &struct{}{}},
	}}}, ""); err != nil {
		return nil, err
	}
	edge := !o.EdgeToken.IsZero()
	if svc := httpServices(o.Services, edge); len(svc.HTTP.Routers) > 0 {
		if err := dyn("10-hashi-http-services.yml", svc, ""); err != nil {
			return nil, err
		}
	} else {
		// Traefik v3.7 rejects an empty `http: {}` and then drops every file of the provider (seen in the integration
		// test); a file with only comments loads as empty.
		files = append(files, File{Path: o.Paths.Dynamic() + "/10-hashi-http-services.yml", Kind: KindDynamic, Mode: 0o644,
			Data: []byte(generated + "# No services are enabled.\n")})
	}
	// Forward auth with the local fallback (06 §2.1, captured construct): fail-open middlewares call the loopback
	// hashi-fa entry point, whose router fails over from Hashi (health-checked on /readyz) to ping@internal (200).
	// Fail-closed middlewares call Hashi directly and get 500 while it is down. The hashi-fa router adds the edge token,
	// so forward-auth requests carry it while upstream services never see it.
	security := &httpConfig{
		Routers: map[string]router{RouterForwardAuth: {
			Rule: "PathPrefix(`/`)", EntryPoints: []string{"hashi-fa"}, Service: ServiceFailover,
		}},
		Middlewares: map[string]middleware{
			MiddlewareAuthOpen: {ForwardAuth: &forwardAuth{
				Address: "http://127.0.0.1:" + strconv.Itoa(o.ForwardAuthPort) + forwardAuthPath, TrustForwardHeader: true}},
			MiddlewareAuthClosed: {ForwardAuth: &forwardAuth{Address: hashi + forwardAuthPath, TrustForwardHeader: true}},
		},
		Services: map[string]service{
			ServiceFailover: {Failover: &failover{Service: ServiceHashi, Fallback: "ping@internal"}},
			ServiceHashi: {LoadBalancer: &loadBalancer{
				Servers:     []server{{URL: hashi}},
				HealthCheck: &healthCheck{Path: readyPath, Interval: "5s", Timeout: "2s"},
			}},
		},
	}
	if edge {
		security.Middlewares[MiddlewareEdge] = edgeMiddleware(o.EdgeToken.Reveal())
		fa := security.Routers[RouterForwardAuth]
		fa.Middlewares = []string{MiddlewareEdge}
		security.Routers[RouterForwardAuth] = fa
	}
	if err := dyn("40-hashi-security.yml", dynamicConfig{HTTP: security}, ""); err != nil {
		return nil, err
	}
	if edge {
		// The file holds the edge token: owner-only, like a key, and kept out of plan text.
		f := &files[len(files)-1]
		f.Mode, f.Secret = 0o600, true
	}
	health := func(mw string) router {
		suffix := "/open"
		if mw == MiddlewareAuthClosed {
			suffix = "/closed"
		}
		return router{Rule: "PathPrefix(`" + HealthPath + suffix + "`)", EntryPoints: []string{"websecure"},
			Middlewares: []string{mw}, Service: "ping@internal", Priority: healthPriority, TLS: &routerTLS{}}
	}
	if err := dyn("90-hashi-health.yml", dynamicConfig{HTTP: &httpConfig{Routers: map[string]router{
		RouterHealthOpen: health(MiddlewareAuthOpen), RouterHealthClosed: health(MiddlewareAuthClosed),
	}}}, ""); err != nil {
		return nil, err
	}

	static, err := marshal(o.static())
	if err != nil {
		return nil, fmt.Errorf("render static config: %w", err)
	}
	files = append(files, logrotateFile(o.Paths))
	files = append(files, File{Path: o.Paths.Static(), Kind: KindStatic, Mode: 0o644, Data: append([]byte(generated), static...)})
	return files, nil
}

// healthPriority keeps the health routers ahead of S5's service routers, whatever their rule length.
const healthPriority = 100000

const generated = "# Generated by Hashi. Changes are overwritten.\n"

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// static is Hashi's static configuration (04 §3), in the shape Traefik v3.7.16 accepted
// (testdata/contracts/traefik/static_capture.yaml).
func (o Options) static() staticConfig {
	loop := func(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }
	websecure := entryPoint{Address: ":443"}
	if len(o.TrustedIPs) > 0 {
		websecure.ForwardedHeaders = &forwardedHeaders{TrustedIPs: append([]string(nil), o.TrustedIPs...)}
	}
	return staticConfig{
		Global: global{CheckNewVersion: false, SendAnonymousUsage: false},
		EntryPoints: map[string]entryPoint{
			"web": {Address: ":80", HTTP: &entryPointHTTP{Redirections: &redirections{
				EntryPoint: redirectEntryPoint{To: "websecure", Scheme: "https"}}}},
			"websecure": websecure,
			"traefik":   {Address: loop(o.APIPort)},
			"hashi-fa": {Address: loop(o.ForwardAuthPort),
				ForwardedHeaders: &forwardedHeaders{TrustedIPs: []string{"127.0.0.1/32", "::1/128"}}},
		},
		API:       api{Insecure: true, Dashboard: false},
		Ping:      ping{EntryPoint: "traefik"},
		Providers: providers{File: fileProvider{Directory: o.Paths.Dynamic(), Watch: true}},
		Log:       logConfig{Format: "json", Level: "INFO", FilePath: LogDir + "/traefik.log"},
		AccessLog: accessLog{Format: "json", FilePath: LogDir + "/access.log"},
	}
}
