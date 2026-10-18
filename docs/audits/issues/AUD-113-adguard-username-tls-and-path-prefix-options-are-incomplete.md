# AUD-113: AdGuard cannot use custom usernames and ignores target TLS modes and path prefixes

Priority: P2

Category: Wrong implementation / AdGuard connection options

Status: open; confirmed by manual source trace.

## Finding

CreateAuthorizedClientAsync always uses admin as the HTTP Basic username and uses the default named HttpClient irrespective of the resolved TlsValidationMode/ExpectedTlsHostname. Configured TLS target behavior is therefore inert. BuildUri stores a path prefix without a trailing slash, while AdGuard operations resolve relative control/... URLs; a base URL ending /adguard resolves to /control/... instead of /adguard/control/.... Valid installations behind a path prefix or using a non-admin login cannot work with these settings.

## Evidence

src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:726-742,793-811 and relative operation URLs at95-96,682-709,954-956; ConnectionTargetResolver.cs:136-146,493 onward. AUD-074 separately covers payload shape incompatibility.

## Expected outcome and acceptance criteria

Represent and use the configured username and documented TLS policy, validate the expected hostname and construct directory-style base URLs safely. Synthetic HTTP requests must retain a configured prefix and expected credentials; TLS acceptance must match the selected policy.
