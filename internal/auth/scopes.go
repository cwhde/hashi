// Package auth owns admin sessions, bootstrap credentials and passkeys (02-setup-auth.md §1.1, §2–3).
package auth

import "slices"

// Scope grants one kind of admin access (02 §3.3). Unknown scopes grant nothing.
type Scope string

const (
	AdminRead      Scope = "admin.read"
	AdminWrite     Scope = "admin.write"
	SettingsManage Scope = "settings.manage"
	SecretsManage  Scope = "secrets.manage"
	SyncApply      Scope = "sync.apply"
	FirewallApply  Scope = "firewall.apply"
	ScriptsManage  Scope = "scripts.manage"
	SecurityManage Scope = "security.manage"
	SetupManage    Scope = "setup.manage" // the only setup.* scope; bootstrap sessions carry nothing else
)

// PasskeyScopes are granted to every passkey session: all non-setup scopes.
var PasskeyScopes = []Scope{AdminRead, AdminWrite, SettingsManage, SecretsManage, SyncApply, FirewallApply, ScriptsManage, SecurityManage}

// BootstrapScopes are granted to bootstrap sessions.
var BootstrapScopes = []Scope{SetupManage}

var known = append(slices.Clone(PasskeyScopes), BootstrapScopes...)

// Known reports whether s is a scope Hashi defines.
func Known(s Scope) bool { return slices.Contains(known, s) }

// HasAll reports whether granted contains every required scope (checks are cumulative). Unknown required scopes are
// never satisfied, even if a row somehow lists them.
func HasAll(granted []Scope, required ...Scope) bool {
	for _, r := range required {
		if !Known(r) || !slices.Contains(granted, r) {
			return false
		}
	}
	return true
}
