# H-016: Missing E2E Tests for Security and CAPTCHA Flows

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §18.3

## Description

The Addendum specifies E2E tests for security features:
```
Add UI tests for:
- Search IP and view timeline.
- Manually block IP.
- Manually allow IP.
- Extend/shorten ban.
- Enable blocklist from setup.
- Add custom blocklist URL.
- Configure Cap integration.
- Trigger challenge page.
- Solve challenge.
- Configure internal agent DNS.
- Configure connection target as Pulse agent.
```

None of these E2E tests exist in the current test suite. The `web/e2e/` directory only contains `setup.spec.ts` with basic setup wizard tests.

## Evidence

The `web/e2e/` directory contains:
```
web/e2e/
└── setup.spec.ts    # Only 4 tests
```

No security, CAPTCHA, blocklist, or agent-related E2E tests exist.

## Expected Outcome

- Security subject search and timeline are tested E2E
- Manual block/allow operations are tested
- Blocklist management is tested
- CAPTCHA challenge flow is tested
- Agent DNS configuration is tested

## Fix Guidance

Add E2E tests for:
1. Security subject search (IP, CIDR, ASN, country)
2. Manual block/allow creation and removal
3. Ban extension/shortening
4. Blocklist enable/disable from setup
5. Custom blocklist URL addition
6. Cap integration configuration
7. Challenge page trigger and solve
8. Internal agent DNS configuration
9. Connection target as Pulse agent

## Acceptance Criteria

- [ ] Security search E2E test exists
- [ ] Manual block/allow E2E tests exist
- [ ] Blocklist management E2E tests exist
- [ ] CAPTCHA challenge E2E test exists
- [ ] Agent DNS configuration E2E test exists
- [ ] Connection target E2E test exists
