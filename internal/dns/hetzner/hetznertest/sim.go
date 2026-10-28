package hetznertest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Sim is a stateful fake of the Hetzner Cloud DNS API for `make dev`, E2E and service tests. It starts from the
// recorded zone (the rrsets_page*.json fixtures) and answers writes in the shapes of the probe capture
// (testdata/contracts/hetzner/write): create, set_records, change_ttl and delete return a "running" Action that the
// next GET /actions/{id} reports as "success"; a label PUT answers the RRSet. Requests it does not model fall through
// to the read replay. It checks what the capture showed the real API rejecting: an existing name+type (409), a
// CNAME beside other types, unquoted TXT and TXT strings over 255 characters (422).
type Sim struct {
	replay *Server
	mu     sync.Mutex
	zones  map[string][]simRRSet // provider zone id → RRSets
	nextID int64
	// Writes counts write requests (anything but GET), for tests.
	Writes []string
	// Fail, when set, may answer a request before the simulation (error-path tests).
	Fail func(w http.ResponseWriter, r *http.Request) bool
}

type simRecord struct {
	Value   string `json:"value"`
	Comment string `json:"comment"`
}

type simRRSet struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	TTL        *int              `json:"ttl"`
	Labels     map[string]string `json:"labels"`
	Protection struct {
		Change bool `json:"change"`
	} `json:"protection"`
	Records []simRecord `json:"records"`
	Zone    int64       `json:"zone"`
}

// NewSim loads the recorded zone from dir (the testdata/contracts/hetzner directory).
func NewSim(dir string) (*Sim, error) {
	replay, err := newServer(dir)
	if err != nil {
		return nil, err
	}
	s := &Sim{replay: replay, zones: map[string][]simRRSet{}, nextID: 900_000_000_000_000}
	pages, err := filepath.Glob(filepath.Join(dir, "rrsets_page*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(pages)
	for _, p := range pages {
		raw, err := os.ReadFile(p) //nolint:gosec // fixture path
		if err != nil {
			return nil, err
		}
		var f struct {
			Body struct {
				RRSets []simRRSet `json:"rrsets"`
			} `json:"body"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for _, r := range f.Body.RRSets {
			id := strconv.FormatInt(r.Zone, 10)
			s.zones[id] = append(s.zones[id], r)
		}
	}
	return s, nil
}

// NewSimServer starts a Sim on a test server; its URL + "/v1" is the adapter base URL.
func NewSimServer(t testing.TB) (*Sim, *httptest.Server) {
	t.Helper()
	s, err := NewSim(Dir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, srv
}

// RRSet returns a copy of the simulated RRSet's record values (nil when absent), for tests.
func (s *Sim) RRSet(zoneID, name, typ string) (values []string, ttl *int, labels map[string]string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.zones[zoneID] {
		if r.Name == name && r.Type == typ {
			for _, rec := range r.Records {
				values = append(values, rec.Value)
			}
			return values, r.TTL, r.Labels, true
		}
	}
	return nil, nil, nil, false
}

// SetProtected marks an RRSet as change-protected, as the console's lock would (tests).
func (s *Sim) SetProtected(zoneID, name, typ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.zones[zoneID] {
		if r.Name == name && r.Type == typ {
			s.zones[zoneID][i].Protection.Change = true
		}
	}
}

func (s *Sim) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Fail != nil && s.Fail(w, r) {
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+Token {
		write(w, s.replay.byKey["401"])
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1"), "/"), "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method != http.MethodGet {
		s.Writes = append(s.Writes, r.Method+" "+r.URL.Path)
	}
	switch {
	case len(parts) == 2 && parts[0] == "actions" && r.Method == http.MethodGet:
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		answer(w, http.StatusOK, map[string]any{"action": action(id, "success", "")})
		return
	case len(parts) >= 3 && parts[0] == "zones" && parts[2] == "rrsets":
		if _, ok := s.zones[parts[1]]; ok || r.Method != http.MethodGet {
			s.rrsets(w, r, parts[1], parts[3:])
			return
		}
	}
	s.replay.serve(w, r)
}

func (s *Sim) rrsets(w http.ResponseWriter, r *http.Request, zone string, rest []string) {
	list := s.zones[zone]
	find := func(name, typ string) int {
		return slices.IndexFunc(list, func(x simRRSet) bool { return x.Name == name && x.Type == typ })
	}
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		answer(w, http.StatusOK, map[string]any{"rrsets": list, "meta": map[string]any{"pagination": map[string]any{
			"page": 1, "per_page": len(list), "previous_page": nil, "next_page": nil, "last_page": 1, "total_entries": len(list)}}})
	case len(rest) == 0 && r.Method == http.MethodPost:
		var b struct {
			Name    string      `json:"name"`
			Type    string      `json:"type"`
			TTL     *int        `json:"ttl"`
			Records []simRecord `json:"records"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, http.StatusBadRequest, "invalid_input", "invalid JSON")
			return
		}
		if find(b.Name, b.Type) >= 0 {
			fail(w, http.StatusConflict, "uniqueness_error", "RRSet(s) already exist(s)")
			return
		}
		for _, x := range list {
			if x.Name == b.Name && (b.Type == "CNAME" || x.Type == "CNAME") {
				fail(w, http.StatusUnprocessableEntity, "invalid_input",
					fmt.Sprintf("(%s, %s) conflicts with (%s, %s)", b.Name, b.Type, x.Name, x.Type))
				return
			}
		}
		if msg := checkTXT(b.Type, b.Records); msg != "" {
			fail(w, http.StatusUnprocessableEntity, "invalid_input", msg)
			return
		}
		zid, _ := strconv.ParseInt(zone, 10, 64)
		n := simRRSet{ID: b.Name + "/" + b.Type, Name: b.Name, Type: b.Type, TTL: b.TTL, Labels: map[string]string{}, Zone: zid}
		for _, rec := range b.Records {
			n.Records = append(n.Records, simRecord{Value: rec.Value})
		}
		s.zones[zone] = append(list, n)
		answer(w, http.StatusCreated, map[string]any{"action": s.newAction("create_rrset"), "rrset": n})
	case len(rest) >= 2:
		i := find(rest[0], rest[1])
		if i < 0 {
			fail(w, http.StatusNotFound, "not_found", "RRSet(s) not found")
			return
		}
		if list[i].Protection.Change && r.Method != http.MethodGet {
			fail(w, http.StatusLocked, "protected", "RRSet is protected") // not captured; the adapter never sends this
			return
		}
		s.change(w, r, zone, i, rest[2:])
	default:
		fail(w, http.StatusNotFound, "not_found", "404 Not Found")
	}
}

func (s *Sim) change(w http.ResponseWriter, r *http.Request, zone string, i int, rest []string) {
	x := &s.zones[zone][i]
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		answer(w, http.StatusOK, map[string]any{"rrset": x})
	case len(rest) == 0 && r.Method == http.MethodPut:
		var b struct {
			Labels map[string]string `json:"labels"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, http.StatusBadRequest, "invalid_input", "invalid JSON")
			return
		}
		x.Labels = b.Labels
		answer(w, http.StatusOK, map[string]any{"rrset": x})
	case len(rest) == 0 && r.Method == http.MethodDelete:
		s.zones[zone] = slices.Delete(s.zones[zone], i, i+1)
		answer(w, http.StatusCreated, map[string]any{"action": s.newAction("delete_rrset")})
	case len(rest) == 2 && rest[0] == "actions" && rest[1] == "set_records":
		var b struct {
			Records []simRecord `json:"records"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, http.StatusBadRequest, "invalid_input", "invalid JSON")
			return
		}
		if msg := checkTXT(x.Type, b.Records); msg != "" {
			fail(w, http.StatusUnprocessableEntity, "invalid_input", msg)
			return
		}
		x.Records = x.Records[:0]
		for _, rec := range b.Records {
			x.Records = append(x.Records, simRecord{Value: rec.Value})
		}
		answer(w, http.StatusCreated, map[string]any{"action": s.newAction("set_rrset_records")})
	case len(rest) == 2 && rest[0] == "actions" && rest[1] == "change_ttl":
		var b struct {
			TTL *int `json:"ttl"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, http.StatusBadRequest, "invalid_input", "invalid JSON")
			return
		}
		x.TTL = b.TTL
		answer(w, http.StatusCreated, map[string]any{"action": s.newAction("change_rrset_ttl")})
	default:
		fail(w, http.StatusNotFound, "not_found", "404 Not Found")
	}
}

// checkTXT mirrors write/create_txt_unquoted.json and write/create_txt_long.json.
func checkTXT(typ string, recs []simRecord) string {
	if typ != "TXT" {
		return ""
	}
	for _, rec := range recs {
		v := strings.TrimSpace(rec.Value)
		if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) {
			return "invalid value '" + rec.Value + "'"
		}
		for _, part := range strings.Split(v, `" "`) {
			if len(strings.Trim(part, `"`)) > 255 {
				return "a TXT string exceeds 255 characters"
			}
		}
	}
	return ""
}

func (s *Sim) newAction(command string) map[string]any {
	s.nextID++
	return action(s.nextID, "running", command)
}

func action(id int64, status, command string) map[string]any {
	progress := 0
	if status == "success" {
		progress = 100
	}
	return map[string]any{"id": id, "status": status, "command": command, "progress": progress,
		"started": "2026-10-28T08:39:10Z", "finished": nil, "error": nil, "resources": []any{}}
}

func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	answer(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": nil}})
}
