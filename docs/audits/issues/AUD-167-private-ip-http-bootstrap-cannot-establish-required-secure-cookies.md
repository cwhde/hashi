# AUD-167: Private IP HTTP bootstrap cannot establish required secure cookies

Priority: P1

Category: Installability / bootstrap transport

Status: open; source and browser cookie policy cross-check; no production browser/server changed.

## Finding and evidence

The first-run workflow permits private/internal network access before the final certificate/domain is configured. The deployment initially serves the admin app over plain HTTP, while auth cookies always require Secure and antiforgery is also configured for secure transport. Browsers do not accept/send Secure cookies from plain HTTP private-IP origins; localhost has a special exception and does not prove LAN bootstrap works. Thus a user opening `http://192.168.x.x:<admin-port>` can submit credentials but cannot retain the authenticated wizard session. Final HTTPS is configured inside that wizard, creating an additional first-install transport dependency.

Evidence: `src/Hashi.Api/Program.cs:74-84` and antiforgery configuration; `deploy/compose/docker-compose.yml`; main specification7.1 permits the private ranges and requires final HTTPS before completion. [Browser Secure cookie policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie#secure).

## Expected outcome and acceptance criteria

Provide an explicit usable secure bootstrap path, such as initial TLS or a documented supported local tunnel/termination flow, while preserving production secure-cookie policy. Verify both fresh localhost and remote private-network installation; do not “fix” this by globally weakening authentication cookie security.
