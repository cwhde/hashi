# Taste

Hashi's own list for `unslop-ui`, taken from the UI rules (`docs/spec/10c-ui-rules.md`, §4 Text) and the owner's bug
reports about text and look (bugs 93, 106, 110, 115, 140, 141, 145, 147). Where this and 10c disagree, fix both in the
same change.

## Bans

- Exclamation marks
- Uppercase text, not only eyebrows (10c §4.3: no all-caps labels)
- Middle dots between items
- A description line under a page title on a detail page, or under a panel title, unless it states a fact the content
  cannot show (a unit, a scope like "every Traefik host") (10c §4.1, §4.2; bugs 93, 106)
- Sentences that explain how Hashi works inside: nftables tables, DNS-01, "Hashi renders", "Hashi requests them itself" (10c §4.4)
- Edge cases written as permanent text; show them where and when they happen, next to the affected value (10c §4.4)
- Reassurance the controls already give ("Nothing changes until you approve" next to "Review plan") (10c §4.4)
- Leftovers from building it: slice names, "arrives later", promises already kept (10c §4.4; bug 61)
- A line under a field that states a limit; the limit shows as the validation message when it is broken (10c §4.5)
- Instructions added to a tag or value ("system · change it in Settings → General" where "system" is enough) (bug 110)
- Labels the owner has to decode: raw seconds ("(s)", 2592000), "… burst", "(0: until solved)"; say the unit or the
  meaning in words (bugs 140, 141)
- A second sentence that adds nothing ("6 services are down. Others are working.") (bug 145)
- Counts and status in a place that is not about status (up/down counts on the app dashboard) (bug 147)
- Any help line, sub-label or tooltip on a button, switch, checkbox, menu item or option that says what it does, when to
  use it or what it doesn't do ("Skip the internal certificate check" + "For an upstream with its own HTTPS
  certificate…"; "Test now" + "Runs both checks once with these settings; saves nothing.") (10c §4.9)
- A state, tag or value followed by what it already implies ("Degraded · slow, still up", "Paused · not checked") (10c §4.10)
- Raw event names, IDs or `kind:<id>` strings where a name or plain words fit (10c §1.6; bugs 129, 131)

## Keep

- Browser tab titles "<Page> · Hashi" (10c, 13.2 gate)
- "More options · N changed" on a closed section (10c §3.1)
- Rows that join real values with " · " (an agent's address · hostname · version): every part is data from the API.
  Cut a part only when it is not data (a hint, an instruction)
- Status badges and words bound to state: ok, waiting, failing, off, "Published", "Waiting for approval" (10c §2)
- Loading placeholders that pulse (`animate-pulse` on skeletons)
- The 橋 mark and the name Hashi
- Approved design mocks under `web/src/routes/design/` are reference, not product: report there, do not edit
