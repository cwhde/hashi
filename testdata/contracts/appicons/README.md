captured_at: 2026-11-20 (read-only GETs from the laptop against the owner's dashboard apps; names replaced)
What an app's public page says about its icon. `*-head.html` is the page's <head> after redirects;
`*-manifest.json` is the web app manifest it links. Gitea (svg icon + manifest), Seerr (sized png links + manifest),
Arcane (manifest with sized icons), Proxmox VE (128 px icon only), 4get (favicon.ico only).
`sonarr-401-headers.txt`: an app behind basic auth answers 401 text/plain, no page.
`nextcloud-redirect-headers.txt`: an app answering 302 to its own login page (same host).
Of 54 dashboard apps: 33 name icons, 15 answer 401/403, 1 redirects to the sign-in host, 4 fail (404/502).
docs: https://html.spec.whatwg.org/multipage/links.html#rel-icon, https://www.w3.org/TR/appmanifest/#icons-member
