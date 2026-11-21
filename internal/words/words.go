// Package words writes counts the way the UI reads them: "1 change", "3 changes", never "3 change(s)" (10c §4).
package words

import "strconv"

// Count joins n with the singular or the plural noun.
func Count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
