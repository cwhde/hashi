# AUD-060: DNS planner treats distinct TXT payloads as equivalent

Priority: P2

Category: DNS correctness / exact record data

Spec reference: Base §§15.2,15.5.

## Finding

DnsPlanner uses case-insensitive dictionaries/grouping and Normalize(value).TrimEnd(dot) for TXT value keys and comparisons. Unlike domain names, TXT data is arbitrary case-sensitive text. Values such as TokenAbc and tokenabc, or value and value-dot, collapse to one key or are reported unchanged. Updates to exact verification tokens and distinct multi-value TXT records can therefore be omitted or conflated.

## Evidence

- src/Hashi.Core/Dns/DnsPlanner.cs:13-16 groups all keys OrdinalIgnoreCase; lines50 and138-149 compare normalized values without type-specific semantics.

## Expected outcome

Use record-type-specific normalization: normalize domain-name fields while preserving TXT payload bytes and case.

## Acceptance criteria

- TXT case-only and trailing-dot changes are planned/applied.
- Distinct TXT values coexist without collapse.
- CNAME/MX hostname normalization remains idempotent according to DNS semantics.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
