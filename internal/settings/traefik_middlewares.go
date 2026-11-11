package settings

import (
	"slices"

	"git.juzo.io/juzo/hashi/internal/usermw"
)

// TraefikMiddlewares is the Traefik setting of the user middleware editor (09 §2): middlewares from the user file
// added to every user HTTP service, after Hashi's defaults and before the service's own (04 §1.1a). System services
// never get them (invariant 10). Edited next to the file on the Traefik page; that the names exist in the file is
// checked where the file is (usermw), since a setting alone cannot see it.
type TraefikMiddlewares struct {
	EveryService []string `json:"every_service"`
}

func (TraefikMiddlewares) Key() string { return "traefik_middlewares" }

func (t TraefikMiddlewares) Validate() error {
	if len(t.EveryService) > 20 {
		return invalid("at most 20 middlewares on every service")
	}
	for i, n := range t.EveryService {
		if err := usermw.CheckName(n); err != nil {
			return invalid("every service: %s", err)
		}
		if slices.Index(t.EveryService, n) != i {
			return invalid("every service: %s is listed twice", n)
		}
	}
	return nil
}
