package dns

import (
	"fmt"
	"slices"
)

// Decision is what the admin chose for an existing RRSet on the import table (03 §4).
type Decision string

const (
	Import Decision = "import" // becomes a managed manual entry with origin "imported"
	Leave  Decision = "leave"  // unmanaged; the default
	Prune  Decision = "prune"  // goes into a separate, confirmed prune plan (S3)
)

// Valid reports whether d is a known decision.
func (d Decision) Valid() bool { return d == Import || d == Leave || d == Prune }

// AllowedDecisions lists what the import table may offer for r. NS and SOA can only be left alone (invariant 2), and
// so can RRSets the provider protects against change and types it does not report as manageable (03 §3): Hashi could
// neither manage nor delete them.
func AllowedDecisions(r RRSet, c Capabilities) []Decision {
	if Protected(r.Type) || r.Protected || !slices.Contains(c.ManagedTypes, r.Type) {
		return []Decision{Leave}
	}
	return []Decision{Import, Leave, Prune}
}

// CheckDecision returns an error when d is not allowed for r.
func CheckDecision(r RRSet, c Capabilities, d Decision) error {
	if !d.Valid() {
		return fmt.Errorf("unknown decision %q", d)
	}
	if slices.Contains(AllowedDecisions(r, c), d) {
		return nil
	}
	why := "the provider protects it against changes"
	switch {
	case Protected(r.Type):
		why = r.Type + " records are never changed or deleted by Hashi"
	case !slices.Contains(c.ManagedTypes, r.Type):
		why = "the DNS provider does not support managing " + r.Type + " records through Hashi"
	}
	return fmt.Errorf("%s %s cannot be marked %q: %s", r.Name, r.Type, d, why)
}
