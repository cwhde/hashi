package feeds

import "net/netip"

// reserved are the ranges a feed may never contribute (06 §8): bound to the global block they would cut off internal
// clients (FireHOL Level 1 lists 10.0.0.0/8 and 100.64.0.0/10, finding 1). A prefix that overlaps one of them is
// rejected as a whole: a /3 that covers multicast is not taken in part.
var reserved = func() []struct {
	p      netip.Prefix
	reason string
} {
	out := []struct {
		p      netip.Prefix
		reason string
	}{}
	add := func(reason string, ps ...string) {
		for _, s := range ps {
			out = append(out, struct {
				p      netip.Prefix
				reason string
			}{netip.MustParsePrefix(s), reason})
		}
	}
	add("private range", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7")
	add("loopback", "127.0.0.0/8", "::1/128")
	add("link-local", "169.254.0.0/16", "fe80::/10")
	add("shared address space (CGNAT, overlay networks)", "100.64.0.0/10")
	add("multicast", "224.0.0.0/4", "ff00::/8")
	add("reserved range", "0.0.0.0/8", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32", "::/128", "::ffff:0:0/96", "64:ff9b:1::/48",
		"100::/64", "2001:db8::/32", "2001::/23")
	return out
}()

// ReservedReason names why p may not come from a feed, or "" when it may.
func ReservedReason(p netip.Prefix) string {
	for _, r := range reserved {
		if r.p.Addr().Is4() == p.Addr().Is4() && r.p.Overlaps(p) {
			return r.reason
		}
	}
	return ""
}

// Filter drops the reserved prefixes, recording each as a reject.
func Filter(in Parsed) Parsed {
	out := Parsed{Ignored: in.Ignored, Rejects: in.Rejects, Rejected: in.Rejected}
	for _, p := range in.Prefixes {
		if why := ReservedReason(p); why != "" {
			out.Rejected++
			if len(out.Rejects) < MaxRejectsKept {
				out.Rejects = append(out.Rejects, Reject{Text: p.String(), Reason: why})
			}
			continue
		}
		out.Prefixes = append(out.Prefixes, p)
	}
	return out
}
