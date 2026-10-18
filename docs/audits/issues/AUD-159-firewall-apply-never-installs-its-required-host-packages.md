# AUD-159: Firewall apply never installs its required host packages

Priority: P1

Category: Missing feature / fresh firewall installation

Status: open; confirmed by manual source trace.

## Finding

The specification says the generated firewall script ensures distribution-specific iptables/ipset/persistence packages. RunPreflightAsync only rejects a host missing commands, and neither the renderer nor the apply path installs those dependencies. A fresh supported host therefore requires undocumented manual preparation before Hashi can manage it. Bash-based validation/execution and cron fallbacks also need explicit distribution capability handling, particularly on Alpine; merely creating /etc/cron.d does not establish a working scheduler.

## Evidence

Main specification14.3 lines717-724; src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:668-685 and renderer complete source; script validation uses bash at737/746. No firewall package-install path found in the manually read implementation.

## Expected outcome and acceptance criteria

Implement reviewed distribution-aware prerequisite provisioning, including the real persistence/scheduler and interpreter requirements, before policy activation. On disposable fresh Debian/Ubuntu/Alpine hosts, first setup must either provision supported prerequisites or give an explicit actionable unsupported-capability result without partial policy changes.
