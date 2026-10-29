// Replays NetBird v0.81.3 client/firewall/create_linux.go check() verbatim
// (same libraries and versions) to see which backend NetBird would pick.
package main

import (
	"fmt"
	"os"

	"github.com/coreos/go-iptables/iptables"
	"github.com/google/nftables"
)

func check() string {
	useIPTABLES := false
	var iptablesChains []string
	ip, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err == nil {
		if _, e := ip.ListChains("filter"); e == nil {
			major, minor, _ := ip.GetIptablesVersion()
			if major < 1 || (major == 1 && minor < 8) {
				return "IPTABLES"
			}
			useIPTABLES = true
			iptablesChains, err = ip.ListChains("filter")
			if err != nil {
				useIPTABLES = false
			}
		}
	}
	if os.Getenv("NB_SKIP_NFTABLES_CHECK") != "true" {
		nf := nftables.Conn{}
		if chains, err := nf.ListChains(); err == nil {
			if !useIPTABLES {
				return "NFTABLES"
			}
			for _, chain := range chains {
				if chain.Table.Name == "filter" {
					return "NFTABLES"
				}
			}
			tl, err := nf.ListTables()
			switch {
			case err == nil && len(iptablesChains) > 0:
				return "IPTABLES"
			case err == nil && len(tl) != 1:
				return "NFTABLES"
			case err == nil && len(tl) == 1 && tl[0].Name == "filter":
				return "IPTABLES"
			}
		}
	}
	if useIPTABLES {
		return "IPTABLES"
	}
	return "UNKNOWN"
}

func main() { fmt.Println(check()) }
