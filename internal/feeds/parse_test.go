package feeds

import (
	"net/netip"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/contracts/feeds/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func entries(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	seen := map[string]bool{} // the sanitized fixtures repeat a replaced address now and then
	for _, l := range strings.Split(string(fixture(t, name)), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, ";") && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// Every captured feed parses with the format Hashi recommends for it, and nothing in it is rejected as malformed.
func TestParseCapturedFeeds(t *testing.T) {
	cases := []struct {
		file   string
		format string
		o      Options
		want   int
		has    string
	}{
		{"feodo_recommended.txt", FormatLines, Options{}, len(entries(t, "feodo_recommended.txt")), ""},
		{"spamhaus_drop.txt", FormatLines, Options{}, len(entries(t, "spamhaus_drop.txt")), ""},
		{"spamhaus_dropv6.txt", FormatLines, Options{}, len(entries(t, "spamhaus_dropv6.txt")), "2001:db8"},
		{"firehol_level1.netset", FormatLines, Options{}, len(entries(t, "firehol_level1.netset")), "10.0.0.0/8"},
		{"spamhaus_drop_v4.jsonl", FormatJSONLines, Options{Field: "cidr"}, len(entries(t, "spamhaus_drop_v4.jsonl")) - 1, ""},
		{"dshield_block.txt", FormatCSV, Options{Column: 1, PrefixColumn: 3}, 0, "/24"},
	}
	for _, c := range cases {
		p, err := Parse(c.format, c.o, fixture(t, c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if p.Rejected != 0 {
			t.Errorf("%s: rejects %+v", c.file, p.Rejects)
		}
		if c.want > 0 && len(p.Prefixes) != c.want {
			t.Errorf("%s: %d prefixes, want %d", c.file, len(p.Prefixes), c.want)
		}
		if len(p.Prefixes) == 0 {
			t.Errorf("%s: nothing parsed", c.file)
		}
		joined := ""
		for _, x := range p.Prefixes {
			joined += x.String() + " "
		}
		if c.has != "" && !strings.Contains(joined, c.has) {
			t.Errorf("%s: no %s in %s", c.file, c.has, joined)
		}
	}
}

// FireHOL Level 1 carries the bogons (finding 1): the filter takes none of them, with the reason.
func TestFilterRejectsReservedRanges(t *testing.T) {
	p, err := Parse(FormatLines, Options{}, fixture(t, "firehol_level1.netset"))
	if err != nil {
		t.Fatal(err)
	}
	f := Filter(p)
	for _, x := range f.Prefixes {
		if ReservedReason(x) != "" {
			t.Errorf("kept %s", x)
		}
	}
	reasons := map[string]string{}
	for _, r := range f.Rejects {
		reasons[r.Text] = r.Reason
	}
	for pfx, want := range map[string]string{"10.0.0.0/8": "private range", "100.64.0.0/10": "shared address space",
		"127.0.0.0/8": "loopback", "224.0.0.0/3": "multicast", "0.0.0.0/8": "reserved range"} {
		if !strings.HasPrefix(reasons[pfx], want) {
			t.Errorf("%s: %q, want %q", pfx, reasons[pfx], want)
		}
	}
	for s, ok := range map[string]bool{"8.8.8.0/24": true, "45.0.0.0/8": true, "0.0.0.0/0": false, "fd00::/8": false,
		"2a00::/12": true, "::ffff:10.0.0.1/128": false, "100.63.255.0/24": true, "100.64.0.1/32": false} {
		p, err := ParsePrefix(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if got := ReservedReason(p) == ""; got != ok {
			t.Errorf("%s (as %s): allowed %v, want %v", s, p, got, ok)
		}
	}
}

func TestParseFormatsAndErrors(t *testing.T) {
	p, err := Parse(FormatLines, Options{}, []byte("# c\n192.0.2.1\n192.0.2.1/32 ; dup\n2001:DB8::1\n::ffff:192.0.2.9\n10.1.2.3/8\nnot-an-ip\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Prefixes) != 4 || p.Rejected != 1 || p.Rejects[0].Line != 7 || p.Ignored != 2 {
		t.Fatalf("lines: %+v", p)
	}
	if p.Prefixes[0] != netip.MustParsePrefix("10.0.0.0/8") {
		t.Errorf("host bits not cleared or not sorted: %v", p.Prefixes)
	}
	if p, _ := Parse(FormatJSONArray, Options{}, []byte(`["192.0.2.1", 5, "x"]`)); len(p.Prefixes) != 1 || p.Rejected != 2 {
		t.Errorf("array: %+v", p)
	}
	if p, _ := Parse(FormatJSONObject, Options{Field: "ips"}, []byte(`{"ips":["192.0.2.0/24"]}`)); len(p.Prefixes) != 1 {
		t.Errorf("object: %+v", p)
	}
	if p, _ := Parse(FormatCSV, Options{Separator: "comma", Column: 2}, []byte("name,address\nx,192.0.2.4\ny,bad\n")); len(p.Prefixes) != 1 || p.Rejected != 1 || p.Ignored != 1 {
		t.Errorf("csv: %+v", p)
	}
	for _, c := range []struct {
		f string
		o Options
		b string
	}{{FormatJSONArray, Options{}, "<html>"}, {FormatJSONObject, Options{Field: "ips"}, "[]"}, {FormatJSONLines, Options{Field: "x"}, "nope\nnope"}} {
		if _, err := Parse(c.f, c.o, []byte(c.b)); err == nil {
			t.Errorf("%s accepted %q", c.f, c.b)
		}
	}
	if err := Validate(FormatCSV, Options{}); err == nil {
		t.Error("CSV without a column")
	}
	if err := Validate("suricata", Options{}); err == nil {
		t.Error("unknown format")
	}
}
