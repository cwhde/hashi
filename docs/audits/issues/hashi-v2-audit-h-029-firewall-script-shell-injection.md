# H-029: FirewallScriptRenderer Shell Injection Risk via Unsanitized User Inputs

**Priority:** Critical
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §14.3, Non-Negotiable Rule Set §3 (#18 custom scripts as privileged operations)

## Description

`FirewallScriptRenderer.Render()` in `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs` directly interpolates user-provided strings into generated bash scripts without any sanitization or escaping. Multiple fields from the `FirewallHostDefinition` record — including `Name`, `Domain`, `InternalTraefikIp`, `NetBirdInterface`, `WanInterface`, `PublicIp`, and target host/port fields in `FirewallPortForward` — are injected directly into shell script strings using bash variable assignment syntax and `iptables` command arguments.

A malicious or accidentally malformed value (e.g., `Name = "evil; rm -rf / #"` or `Domain = "example.com\" && echo pwned"`) could produce a firewall script that executes arbitrary commands with root privileges when Hashi applies firewall state via SSH to a remote host.

## Evidence

```csharp
// FirewallScriptRenderer.cs lines 48-56
return $$"""
    #!/bin/bash
    # Hashi-managed firewall script for {{host.Name}} (spec section 14)
    set -euo pipefail

    ROLLBACK_TIMER={{rollbackTimer}}
    WAN_IF="{{wan}}"
    PUBLIC_IP="{{publicIp}}"
    TRAEFIK_IP="{{host.InternalTraefikIp}}"
    NETBIRD_IF="{{host.NetBirdInterface}}"
    ...
""";
```

```csharp
// Lines 40-43 — port forward DNAT/FWD rules with unsanitized Protocol, TargetHost, TargetPort, PublicPort
var dnatRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
    $"iptables -t nat -A HASHI_DNAT -p {p.Protocol.ToLowerInvariant()} --dport {p.PublicPort} -j DNAT --to-destination {p.TargetHost}:{p.TargetPort}"));
var fwdRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
    $"iptables -A HASHI_FWD -p {p.Protocol.ToLowerInvariant()} -d {p.TargetHost} --dport {p.TargetPort} -j ACCEPT"));
```

None of the interpolated values are shell-escaped, single-quoted, or validated against shell metacharacters before injection.

## Expected Outcome

All user-provided string values interpolated into generated shell scripts must be:
1. Validated against shell metacharacter patterns before insertion
2. Single-quoted (with proper single-quote escaping: replace `'` with `'\''`)
3. Rejected with a validation error if they contain characters inconsistent with their field type (e.g., `Name` should not contain `/`, `;`, `$`, backticks)

## Fix Guidance

1. Add a `ShellEscape(string value)` helper that wraps values in single quotes with internal single-quote escaping.
2. Add input validation at the `FirewallHostDefinition` record level or at the `FirewallScriptRenderer.Render()` method entry that rejects values containing dangerous characters.
3. Alternatively, use a structured bash script generation approach (e.g., heredoc with properly quoted variables) rather than string interpolation.
4. Ensure that `Port`, `PublicPort`, and `TargetPort` are validated as integers before interpolation (they come as `int` fields which is safe, but verify there's no string path).

## Acceptance Criteria

- [ ] Generated firewall script successfully passes `shellcheck` analysis
- [ ] Test case: A host name containing `$(whoami)` or backticks does not produce executable injection in the output script
- [ ] Test case: A domain containing `"` or `'` does not break script syntax
- [ ] All string fields injected into the script template are escaped or validated
- [ ] `FirewallScriptRendererTests.Render_passes_shellcheck_when_available` test passes with sanitized output
