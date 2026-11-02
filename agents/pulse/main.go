// Command pulse reports this machine's current addresses to Hashi (08 §1). It holds one token that can only send this
// agent's heartbeat, keeps no local state and writes nothing to disk. Between beats it replaces itself with a tiny
// sleeper, so memory is used only for the moment of a beat; net/http and encoding/json stay out for the same reason.
package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Version is set at build time.
var Version = "dev"

const defaultConfig = "/etc/hashi-pulse/config.yaml"

type config struct {
	URL, AgentID, Token, Interface string
	Interval                       time.Duration
	// OwnInterval: the config names an interval, so Hashi's default does not replace it.
	OwnInterval bool
	// Resident keeps the process in memory between beats, for a machine that forbids running the sleeper.
	Resident bool
}

// readConfig takes flat "key: value" lines from path, then environment variables over them. Nothing else in the format
// is used, so no YAML library is needed in a binary meant to stay small.
func readConfig(path string, env func(string) string) (config, error) {
	vals := map[string]string{}
	f, err := os.Open(path) //nolint:gosec // the admin's own config path
	switch {
	case err == nil:
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, ":"); ok {
				vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
		if err := sc.Err(); err != nil {
			return config{}, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return config{}, err
	}
	for key, name := range map[string]string{"url": "HASHI_URL", "agent_id": "HASHI_AGENT_ID", "token": "HASHI_TOKEN", "interval": "HASHI_INTERVAL", "interface": "HASHI_INTERFACE"} {
		if v := env(name); v != "" {
			vals[key] = v
		}
	}
	c := config{URL: strings.TrimRight(vals["url"], "/"), AgentID: vals["agent_id"], Token: vals["token"], Interface: vals["interface"], Interval: time.Minute, Resident: env("HASHI_PULSE_RESIDENT") != ""}
	if v := vals["interval"]; v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 10 {
			return c, fmt.Errorf("interval must be at least 10 seconds, got %q", v)
		}
		c.Interval, c.OwnInterval = time.Duration(secs)*time.Second, true
	}
	if c.URL == "" || c.AgentID == "" || c.Token == "" {
		return c, errors.New("url, agent_id and token are required")
	}
	return c, nil
}

type candidate struct{ Interface, Address string }

type beat struct {
	Version, Hostname string
	Candidates        []candidate
	Selected          string
	Interval          int
	SentAt            time.Time
	InDocker          bool
	// ReportedPublic is this machine's public address as outside services see it. Sent only when Hashi asks for it.
	ReportedPublic string
}

// reply is Hashi's answer to a heartbeat.
type reply struct {
	CheckPublicIP   bool
	IntervalSeconds int
}

// Outside services are asked by address, not name: a filtering DNS server blocks the well-known names
// (testdata/contracts/public-ip). They are two separate companies, one over authenticated TLS and one over DNS.
const (
	traceURL        = "https://1.1.1.1/cdn-cgi/trace"
	openDNSServer   = "208.67.222.222:53"
	openDNSName     = "myip.opendns.com"
	publicIPRecheck = 5 * time.Minute
)

// parseTrace finds the address in Cloudflare's trace answer.
func parseTrace(body string) (netip.Addr, error) {
	for _, l := range strings.Split(body, "\n") {
		if v, ok := strings.CutPrefix(l, "ip="); ok {
			return netip.ParseAddr(strings.TrimSpace(v))
		}
	}
	return netip.Addr{}, errors.New("no address in the answer")
}

func traceIP(ctx context.Context) (netip.Addr, error) {
	res, err := roundTrip(ctx, request{Method: "GET", URL: traceURL}, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	if res.Status != 200 {
		return netip.Addr{}, fmt.Errorf("the address check answered HTTP %d", res.Status)
	}
	return parseTrace(res.Body)
}

func openDNSIP(ctx context.Context) (netip.Addr, error) {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, openDNSServer)
	}}
	addrs, err := r.LookupNetIP(ctx, "ip4", openDNSName)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(addrs) != 1 {
		return netip.Addr{}, errors.New("expected one address")
	}
	return addrs[0].Unmap(), nil
}

// agreed returns the address only when both sources name the same public one: one wrong answer must not move a firewall rule.
func agreed(a, b netip.Addr) (string, bool) {
	if !a.IsValid() || a != b || !a.IsGlobalUnicast() || a.IsPrivate() {
		return "", false
	}
	return a.String(), true
}

func publicIP(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	a, err := traceIP(ctx)
	if err != nil {
		log.Printf("public address check: %v", err)
		return ""
	}
	b, err := openDNSIP(ctx)
	if err != nil {
		log.Printf("public address check: %v", err)
		return ""
	}
	ip, ok := agreed(a, b)
	if !ok {
		log.Print("public address check: the two sources disagree, not reporting")
	}
	return ip
}

// usable leaves out what could never reach this machine from elsewhere.
func usable(a netip.Addr) bool {
	return a.IsValid() && !a.IsLoopback() && !a.IsLinkLocalUnicast() && !a.IsMulticast() && !a.IsUnspecified()
}

func candidates() ([]candidate, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := []candidate{}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, x := range addrs {
			p, err := netip.ParsePrefix(x.String())
			if err == nil && usable(p.Addr().Unmap()) {
				out = append(out, candidate{Interface: i.Name, Address: p.Addr().Unmap().String()})
			}
		}
	}
	return out, nil
}

// send delivers one heartbeat. root, when given, is the only certificate trusted for it.
func send(ctx context.Context, c config, b beat, root *x509.CertPool) (reply, *response, error) {
	var out reply
	res, err := roundTrip(ctx, request{Method: "POST", URL: c.URL + "/api/pulse/" + c.AgentID + "/heartbeat", Bearer: c.Token, Body: b.json()}, root)
	if err != nil {
		return out, nil, err
	}
	if res.Status != 200 {
		return out, nil, fmt.Errorf("Hashi answered HTTP %d", res.Status) //nolint:staticcheck // a name
	}
	out, err = parseReply(res.Body)
	return out, res, err
}

// inDocker reports whether this process runs in a Docker container.
func inDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// jitter spreads retries over time: ±10 %.
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d/5)+1)) - d/10 //nolint:gosec // not security relevant
}

// slotWait is the time until this agent's next slot. Each agent beats at a fixed point of the interval, taken from its
// ID, so a whole server starting at once, or restarting, never wakes up together and no agent has to know of another.
func slotWait(id string, interval time.Duration, now time.Time) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	ms := interval.Milliseconds()
	return time.Duration((int64(h.Sum64()%uint64(ms))-now.UnixMilli()%ms+ms)%ms) * time.Millisecond //nolint:gosec // ms is positive
}

// nextBeat is the next slot, at least half an interval away so that a change of interval cannot make two beats touch.
func nextBeat(id string, interval time.Duration, now time.Time) time.Duration {
	w := slotWait(id, interval, now)
	if w < interval/2 {
		w += interval
	}
	return w
}

// rootMaxAge is how long a beat trusts the one root it kept, so a root removed from the machine's list stops being used.
const rootMaxAge = 24 * time.Hour

// beatOnce sends one heartbeat, keeping in st only the root that vouched for Hashi and going back to the full list when
// that root no longer does. It returns the time to wait.
func beatOnce(ctx context.Context, c *config, st *state, host string) time.Duration {
	if !c.OwnInterval && st.Interval >= 10 {
		c.Interval = time.Duration(st.Interval) * time.Second
	}
	if !st.Check {
		st.Reported = ""
	} else if time.Since(time.Unix(st.Checked, 0)) >= publicIPRecheck { // outside services hear from this machine only this often
		st.Reported, st.Checked = publicIP(ctx), time.Now().Unix()
	}
	cs, err := candidates()
	var r reply
	if err == nil {
		r, err = deliver(ctx, *c, st, beat{Version: Version, Hostname: host, Candidates: cs, Selected: c.Interface, Interval: int(c.Interval.Seconds()), SentAt: time.Now().UTC(), InDocker: inDocker(), ReportedPublic: st.Reported})
	}
	if err != nil {
		st.Failures++
		wait := jitter(min(c.Interval, 5*time.Second<<min(st.Failures-1, 8)))
		log.Printf("heartbeat failed, retrying in %s: %q", wait.Round(time.Second), err.Error()) //nolint:gosec // a duration and a quoted error: nothing can start a new log line
		return wait
	}
	st.Failures = 0
	checkNow := r.CheckPublicIP && !st.Check
	st.Check = r.CheckPublicIP
	if r.IntervalSeconds >= 10 {
		st.Interval = r.IntervalSeconds
		if !c.OwnInterval {
			c.Interval = time.Duration(r.IntervalSeconds) * time.Second
		}
	}
	if checkNow {
		return 0
	}
	return nextBeat(c.AgentID, c.Interval, time.Now())
}

func deliver(ctx context.Context, c config, st *state, b beat) (reply, error) {
	pool := st.pool(time.Now())
	r, res, err := send(ctx, c, b, pool)
	var bad *tlsVerifyError
	if pool != nil && errors.As(err, &bad) { // the kept root no longer fits, say after Hashi changed its certificate authority
		st.Root = nil
		r, res, err = send(ctx, c, b, nil)
	}
	if err == nil && st.Root == nil && res.Root != nil {
		st.Root, st.RootAt = res.Root, time.Now().Unix()
	}
	return r, err
}

// pauser waits between beats and reports whether to go on.
type pauser func(ctx context.Context, wait time.Duration, st state) bool

func residentPause(ctx context.Context, wait time.Duration, _ state) bool {
	debug.FreeOSMemory()
	select {
	case <-ctx.Done():
		return false
	case <-time.After(wait):
		return true
	}
}

// sleepingPause gives up the process to the sleeper, which starts the agent again when the wait is over. If the machine
// refuses that, the agent stays in memory instead: it uses more, and it still works.
func sleepingPause(ctx context.Context, wait time.Duration, st state) bool {
	if ctx.Err() != nil {
		return false
	}
	if err := execSleeper(wait, st); err != nil {
		log.Printf("staying in memory between heartbeats, the sleeper could not start: %v", err)
		sleeperBroken = true
	}
	return residentPause(ctx, wait, st)
}

var sleeperBroken bool

func execSleeper(wait time.Duration, st state) error {
	if len(sleeperImage) == 0 || sleeperBroken {
		return errors.New("not available")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	fd, err := memfd()
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(fd) }()
	if _, err := syscall.Write(fd, sleeperImage); err != nil {
		return err
	}
	env := append(os.Environ(), stateEnv+"="+st.encode())
	return syscall.Exec("/proc/self/fd/"+strconv.Itoa(fd), []string{"hashi-pulse-sleep", strconv.FormatInt(wait.Milliseconds(), 10), self}, env) //nolint:gosec // runs the built-in sleeper, then this same binary
}

// memfd opens an unnamed in-memory file for the sleeper, so the agent stays one file and writes nothing to disk.
func memfd() (int, error) {
	const cloexec, exec = 0x1, 0x10
	name := []byte("hashi-pulse-sleep\x00")
	fd, _, e := syscall.Syscall(sysMemfdCreate, uintptr(unsafe.Pointer(&name[0])), cloexec|exec, 0) //nolint:gosec // the syscall package has no memfd_create
	if e == syscall.EINVAL {                                                                        // kernels before 6.3 do not know the exec flag, and allow it anyway
		fd, _, e = syscall.Syscall(sysMemfdCreate, uintptr(unsafe.Pointer(&name[0])), cloexec, 0) //nolint:gosec // as above
	}
	if e != 0 {
		return -1, e
	}
	return int(fd), nil
}

func run(ctx context.Context, c config, st state, pause pauser) error {
	host, _ := os.Hostname()
	if !c.OwnInterval && st.Interval >= 10 {
		c.Interval = time.Duration(st.Interval) * time.Second
	}
	wait := time.Duration(0)
	if !st.Due { // a fresh start: nothing has been loaded yet, so wake up only at this agent's slot
		wait = slotWait(c.AgentID, c.Interval, time.Now())
	}
	for {
		if wait > 0 {
			st.Due = true
			if !pause(ctx, wait, st) {
				return nil
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		st.Due = false
		wait = beatOnce(ctx, &c, &st, host)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(Version)
		return
	}
	runtime.GOMAXPROCS(1)
	debug.SetGCPercent(25)
	path := defaultConfig
	if v := os.Getenv("HASHI_PULSE_CONFIG"); v != "" {
		path = v
	}
	c, err := readConfig(path, os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pause := sleepingPause
	if c.Resident {
		pause = residentPause
	}
	if err := run(ctx, c, decodeState(os.Getenv(stateEnv)), pause); err != nil {
		log.Fatal(err)
	}
}
