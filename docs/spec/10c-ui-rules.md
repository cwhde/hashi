# 10c — UI rules: where things live, what is shown when, what is written

10b says how things look. This file says **where** a thing belongs, **when** it is shown and **whether** it gets words.
Where 10b and this file disagree, this file wins. Every page change is checked against the checklist in §7.

Why it exists: a survey of the admin app (2026-11-16) found about 200 help sentences in 91 components, a description
under almost every page title and panel, 6 places that fold anything away, settings for one subsystem spread over two or
three places, and the same concept (a target, a save) handled differently per page. Bugs 17, 24, 28, 40, 63–65, 76,
77, 90, 91, 93 and 94 are symptoms of that.

## 1. Where things live

There are exactly three places for anything you can change.

| What it changes | Where it lives | Example |
| --- | --- | --- |
| One object (a service, a host, an IP set, a rule set, an agent, a script, a check) | On that object's page | A service's upstream, a host's ports, an agent's main address |
| How Hashi reaches an outside system (address, credentials, certificate, read-only or not) | **Connections** | DNS provider token, AdGuard login, Traefik and firewall host SSH, Pulse agent tokens |
| A default or behaviour for a whole subsystem | **Settings**, one category per subsystem | Monitor defaults, ban function, CAPTCHA, session lengths |

An object is anything with its own identity, also when it's only a row in a list: an address with its counters, a DNS
record, an AdGuard rewrite, a dynamic entry. Its per-item settings belong where the object is shown, never in Settings.
A feature page that lists such objects (Security → Counters, DNS → Records) edits them in place: an expandable row or
the object's page. Settings holds only what applies to many objects at once.

Rules that follow from it:

1. **Feature pages show and operate; they don't hold global settings.** The Security page shows what happened and lets
   you act on addresses. The engine, CAPTCHA, edge sign-in, limits and GeoIP settings move to Settings. A feature page
   with settings elsewhere gets a gear link in its header to the matching Settings category. Nothing else.
2. **Settings categories carry the name of the page they configure**, in the navigation's order: General, Security
   (admin sign-in, passkeys, vault, sessions, bootstrap networks, trusted proxies), Edge protection (engine, limits,
   CAPTCHA, edge sign-in session lengths), Services and Traefik (middlewares on every service), DNS and agent names,
   Firewall, Pulse, Monitoring and notifications, Public pages (dashboard and status page), Appearance. There is no
   Account category: Hashi stores nothing about the person, and signing out everywhere is in Security's sessions.
   Edge sign-in providers (issuer, client secret) and the GeoIP download source (MaxMind account, licence key) are
   credentials for outside systems, so they live on Connections with the other connections (2026-11-18).
3. **One setting, one place.** A value is never editable in two places. Another place can show it read-only with a link.
4. **Defaults vs. overrides:** the default lives in Settings; an object that overrides it shows the override on its own
   page with "Default: …" next to it and a way back to the default.
5. **History of one object lives on that object** (its History or Activity tab). Activity holds history across all objects.
6. **Every reference to another object is a link** that opens it, and shows its name, never an ID.

## 2. One concept, one control

The same concept looks and behaves the same everywhere. Before building a control, check this list; if the concept is
here, use the shared component; if it's new and used twice, add it here.

| Concept | Control | Used for |
| --- | --- | --- |
| Where something is reached | **Target picker**: Address · Agent · IP set (one switch, then one field) | Service upstream, route targets, AdGuard and every connection, SSH target, DNS values |
| A group of addresses | IP set picker | Firewall allow/block, rules, host access |
| A change to a remote system | Plan review (10b §4) | Pending changes, save confirmations |
| State of a thing | Status badge with one of: ok, waiting, failing, off | Every list and summary strip |
| Nothing there yet | Empty state: one short line + the one action | Every list, tab, panel |
| Options most visits don't need | Disclosure section (§3) | Forms, settings categories, detail tabs |

Status words claim only what is confirmed: "Published" only after every host applied; "Waiting for approval" while a
plan waits.

## 3. Progressive disclosure

Show what most visits need. Everything else is one click away, and you can always tell that it is there.

**Always visible, never folded:** name, status, risk, errors, values changed from the default, the primary action.

**Levels:**

- **Visible:** what you look at or change on most visits.
- **One click:** secondary fields and details. In forms: a closed **More options** section at the end. In lists: an
  expandable row or the object's page. In detail pages: a later tab.
- **Out of the way:** technical and read-only views (rendered config, live ruleset, resolved prefixes, raw JSON) go in the
  last tab or behind **Advanced**.

**Rules:**

1. A closed section shows when something inside differs from the default: "More options · 2 changed".
2. A field that only matters when another is on appears only when that one is on (HTTPS → certificate fields; mode
   "custom" → its value).
3. Create forms ask only for what's needed to create. Everything else is set on the object's page afterwards.
4. Lists show identity, status and one key value per row; secondary columns appear on wide screens or in the expanded row.
5. A tab or panel with nothing in it shrinks to its empty state line; it doesn't keep its full frame.
6. A detail page has a summary strip and at most 5 tabs, ordered by how often they're used. Read-only technical tabs come last.
7. A drawer holds one task. If it needs tabs, it's a page.

## 4. Text

**Default: no text.** The layout, labels, tags and the controls themselves explain the page. A sentence has to earn its
place.

Test for every sentence: delete it. Would someone now make a real mistake or be stuck? If not, it stays deleted.

1. **Page headers:** title only. A description is allowed only on a top-level page whose purpose isn't clear from its
   title and content, one short sentence, never on a detail page.
2. **Panels:** title only. A muted line only for a fact the content can't show (a unit, a scope like "every Traefik host").
3. **No eyebrows.** No small label above a heading or card unless it's data (the object's kind). No all-caps labels.
4. **Never write:**
   - what visible tags, columns or badges already say (DNS: "records it does not own are shown but never changed"
     next to Owned / Unmanaged / Managed tags);
   - how Hashi works inside (nftables tables, DNS-01, "Hashi renders", "Hashi requests them itself");
   - edge cases as permanent text. Show them where and when they happen, as a warning next to the affected value;
   - reassurance that the controls already give ("Nothing changes until you approve" when the button says
     "Review plan"), at most once per flow, inside the plan review;
   - leftovers from building it (slice names, "arrives later", "arrive with monitoring (S7)").
5. **Field help:** label + placeholder first. Format examples go in the placeholder. Limits are shown as the validation
   message when broken, not as a line under the field ("Lowercase letters, digits and dashes" appears only after a
   wrong character). At most one short line, and only for a consequence you can't see ("Fixed after setup").
6. **Consequences at the moment they apply:** "Opening a port is a high-risk change" shows next to Save when the edit
   opens a port, not always.
7. **Concepts that need explaining** (rule order, ban counters) get one help icon per panel with at most three sentences
   in a popover, not a paragraph on the page.
8. **One name per thing** across pages, buttons, toasts and plans: IP set, rule set, agent, plan, apply, approve.
9. **Checked with `unslop-ui`** (13.10): the repo's `.unslop-ui/taste.md` lists this section's bans and what stays on
   purpose (" · " between real values, "<Page> · Hashi" titles). A text change runs it; where it and this section
   disagree, both change in the same commit.

## 5. Saving and flows

1. **Two save models, chosen by what the control does:**
   - **Instant:** switches in lists that change only Hashi itself and can be switched back (show on public pages,
     enable a check, mute an alert). The switch reflects the saved state; a failure flips it back with the error.
   - **Save:** every form, every Settings category, everything that creates a plan. Save is disabled until something
     changed.
2. **The Save button says what happens:** "Save" when it applies at once; "Save and review plan" when the result waits
   for approval. The message afterwards names only what this save produced and links its plan.
3. **Drafts:** switching tabs or Settings categories keeps the draft; leaving the page with unsaved changes asks first.
4. **Create:** "Add …" opens a drawer with only the required fields; after creating, you land on the new object's page.
   Multi-step wizards only for first setup.
5. **Delete:** in the row menu or at the bottom of the object's first tab; the confirmation names exactly what goes and
   which plan it makes.
6. **Errors** sit next to the field or action that caused them and disappear when the input changes.

## 6. What this moves (fix order for the UI bugs)

| Today | Rule | Target |
| --- | --- | --- |
| Security page tabs CAPTCHA, Sign-in, Limits, Engine, GeoIP | §1.1 | Settings → Edge protection (CAPTCHA, session limits, engine); sign-in providers and the GeoIP source on Connections; Security keeps Dashboard and Counters |
| Settings → Account holds passkeys, sessions, vault; Security holds session settings | §1.2 | Settings → Security; no Account category |
| Status → Notifications tab + Settings → Notifications | §1.2, §1.5 | Settings → Monitoring and notifications for setup; deliveries in Activity or on the check |
| DNS → Agent names tab + Settings → Internal agent DNS | §1.3 | Settings for the switch and domain; DNS shows the names read-only |
| App display page switches + Settings → Appearance public pages on/off + Settings → Dashboard | §1.2, §5.1 | Settings → Public pages for page-wide options; per-service visibility stays an instant switch on Public pages |
| Traefik, firewall hosts, agents added on their own pages, AdGuard and DNS on Connections | §1 table | Connections adds every outside system; feature pages link there (bug 24) |
| Service target: address or IP set only | §2 | Target picker with Agent (bug 17) |
| ~200 help sentences, descriptions under titles and panels | §4 | Remove per the test; expected to drop below 40 (bugs 28, 93) |
| Forms showing every option | §3 | More options sections, conditional fields (bugs 90, 94) |
| Mixed save behaviour, lost drafts | §5 | Bugs 40, 65, 76, 91 |

## 7. Checklist for every page change

- [ ] Every setting on the page belongs here by §1, or it moves.
- [ ] Shared concepts use the shared control from §2.
- [ ] Name, status, risk, errors and non-default values are visible; everything else is one click away (§3).
- [ ] Every sentence passed the deletion test; no eyebrows; no description on a detail page (§4).
- [ ] Save model and button label follow §5; drafts survive tab switches; leaving asks.
- [ ] Every reference is a named link.
