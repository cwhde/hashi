package edge

import "strings"

// NormalizePath is the one normalization every path condition sees (06 §2): percent-decoding of unreserved characters,
// dot-segment removal, duplicate slashes collapsed, query dropped. Other escapes stay encoded (a %2F is not a separator).
func NormalizePath(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '%' && i+2 < len(raw) {
			hi, lo := unhex(raw[i+1]), unhex(raw[i+2])
			if hi >= 0 && lo >= 0 {
				d := byte(hi<<4 | lo) //nolint:gosec // both are hex digits, 0–15
				if unreserved(d) {
					b.WriteByte(d)
				} else {
					b.WriteString(strings.ToUpper(raw[i : i+3]))
				}
				i += 2
				continue
			}
		}
		b.WriteByte(c)
	}
	var out []string
	segs := strings.Split(b.String(), "/")
	for _, s := range segs {
		switch s {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, s)
		}
	}
	p := "/" + strings.Join(out, "/")
	if last := segs[len(segs)-1]; len(out) > 0 && (last == "" || last == "." || last == "..") {
		p += "/"
	}
	return p
}

func unreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~'
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
