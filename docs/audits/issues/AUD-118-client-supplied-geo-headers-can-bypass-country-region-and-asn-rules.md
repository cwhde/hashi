# AUD-118: Client supplied geo headers can bypass country region and ASN rules

Priority: P1

Category: Security / forwarded geo metadata

Status: open; confirmed by manual source trace.

## Finding

EdgeAuthEndpoints trusts X-Geo-Country/Region/Asn whenever the immediate peer is a trusted proxy. The generated Traefik forwardAuth configuration has no authRequestHeaders allowlist or middleware stripping/replacing those client headers. Traefik forwards all request headers by default. An external client can therefore send all three geo headers through the legitimate trusted proxy, suppress the local GeoIP lookup, and choose the metadata evaluated by geo rules. Trusting the transport peer does not establish the provenance of these arbitrary headers. This applies to the generated configuration unless another independently configured upstream sanitizes them.

## Evidence

src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs:27-54; src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:215-232; repository search finds no generated X-Geo stripping/producer. Official default behavior: https://doc.traefik.io/traefik/v3.4/middlewares/http/forwardauth/#authrequestheaders . AUD-102 is a separate database capability defect.

## Expected outcome and acceptance criteria

Derive geo data locally from the canonical client IP, or accept it only through an explicit authenticated producer that strips client copies. Test forged geo headers through the actual generated proxy path; effective country/region/ASN must remain the server-derived values.
