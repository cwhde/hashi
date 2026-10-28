package traefik

// Traefik configuration as Go structs (CLAUDE.md: renderers marshal structs, never template YAML). Field names follow
// the captured v3.7.16 files in testdata/contracts/traefik/. Maps marshal with sorted keys, so output is deterministic.

type staticConfig struct {
	Global      global                `yaml:"global"`
	EntryPoints map[string]entryPoint `yaml:"entryPoints"`
	API         api                   `yaml:"api"`
	Ping        ping                  `yaml:"ping"`
	Providers   providers             `yaml:"providers"`
	Log         logConfig             `yaml:"log"`
	AccessLog   accessLog             `yaml:"accessLog"`
}

// global turns off the update check and usage reporting: edges make no outbound calls of their own.
type global struct {
	CheckNewVersion    bool `yaml:"checkNewVersion"`
	SendAnonymousUsage bool `yaml:"sendAnonymousUsage"`
}

type entryPoint struct {
	Address          string            `yaml:"address"`
	HTTP             *entryPointHTTP   `yaml:"http,omitempty"`
	ForwardedHeaders *forwardedHeaders `yaml:"forwardedHeaders,omitempty"`
}

type entryPointHTTP struct {
	Redirections *redirections `yaml:"redirections,omitempty"`
}

type redirections struct {
	EntryPoint redirectEntryPoint `yaml:"entryPoint"`
}

type redirectEntryPoint struct {
	To     string `yaml:"to"`
	Scheme string `yaml:"scheme"`
}

type forwardedHeaders struct {
	TrustedIPs []string `yaml:"trustedIPs"`
}

type api struct {
	Insecure  bool `yaml:"insecure"`
	Dashboard bool `yaml:"dashboard"`
}

type ping struct {
	EntryPoint string `yaml:"entryPoint"`
}

type providers struct {
	File fileProvider `yaml:"file"`
}

type fileProvider struct {
	Directory string `yaml:"directory"`
	Watch     bool   `yaml:"watch"`
}

type logConfig struct {
	Format   string `yaml:"format"`
	Level    string `yaml:"level"`
	FilePath string `yaml:"filePath"`
}

type accessLog struct {
	Format   string `yaml:"format"`
	FilePath string `yaml:"filePath"`
}

type dynamicConfig struct {
	HTTP *httpConfig `yaml:"http,omitempty"`
	TLS  *tlsConfig  `yaml:"tls,omitempty"`
}

type httpConfig struct {
	Routers     map[string]router     `yaml:"routers,omitempty"`
	Middlewares map[string]middleware `yaml:"middlewares,omitempty"`
	Services    map[string]service    `yaml:"services,omitempty"`
}

type router struct {
	Rule        string    `yaml:"rule"`
	EntryPoints []string  `yaml:"entryPoints"`
	Middlewares []string  `yaml:"middlewares,omitempty"`
	Service     string    `yaml:"service"`
	Priority    int       `yaml:"priority,omitempty"`
	TLS         *struct{} `yaml:"tls,omitempty"`
}

type middleware struct {
	Headers     *headers     `yaml:"headers,omitempty"`
	Compress    *struct{}    `yaml:"compress,omitempty"`
	ForwardAuth *forwardAuth `yaml:"forwardAuth,omitempty"`
}

type headers struct {
	STSSeconds           int    `yaml:"stsSeconds,omitempty"`
	STSIncludeSubdomains bool   `yaml:"stsIncludeSubdomains,omitempty"`
	ContentTypeNosniff   bool   `yaml:"contentTypeNosniff,omitempty"`
	ReferrerPolicy       string `yaml:"referrerPolicy,omitempty"`
}

type forwardAuth struct {
	Address            string `yaml:"address"`
	TrustForwardHeader bool   `yaml:"trustForwardHeader"`
}

type service struct {
	LoadBalancer *loadBalancer `yaml:"loadBalancer,omitempty"`
	Failover     *failover     `yaml:"failover,omitempty"`
}

type loadBalancer struct {
	Servers     []server     `yaml:"servers"`
	HealthCheck *healthCheck `yaml:"healthCheck,omitempty"`
}

type server struct {
	URL string `yaml:"url"`
}

type healthCheck struct {
	Path     string `yaml:"path"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
}

type failover struct {
	Service  string `yaml:"service"`
	Fallback string `yaml:"fallback"`
}

type tlsConfig struct {
	Certificates []certFile `yaml:"certificates"`
}

type certFile struct {
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}
