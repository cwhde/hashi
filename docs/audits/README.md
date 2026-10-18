# Hashi audit and implementation backlog

[**Current review and ordered backlog — 2026-10-18**](TRIAGE-2026-10-18.md) is the status authority. Start there for the full manual review, specification gaps, security/correctness findings and the repair sequence.

## Working records

- [Specification matrix](full-review/SPEC-MATRIX.md), [all section dispositions](full-review/spec-sections.csv) and [preserved candidate passages](full-review/requirements.csv).
- [Historical issue reconciliation](full-review/HISTORICAL-RECONCILIATION.md) and [CSV](full-review/historical-issues.csv).
- [Current backlog CSV](full-review/backlog.csv) / [JSON](full-review/backlog.json). Each stable ID links to its detailed finding, evidence and acceptance criteria under `issues/`.
- [Complete baseline file ledger](full-review/coverage.csv) / [JSON with read ranges](full-review/coverage.json).
- [Independent verification and scanner dispositions](full-review/SUPPLEMENTAL-VERIFICATION.md).
- [Branches and PRs](full-review/BRANCH-STATE.md).

## Status rules

Current source evidence controls status. A merged branch proves integration, not correctness. A fixed narrow mechanism can leave a product feature incomplete; use the linked residual finding. False positives remain documented with the rejected premise. Existing tests written alongside the implementation are context, not independent acceptance proof. Do not create duplicate tasks from old unchecked lists or blanket completion summaries.

Original A–H issue write-ups and audit narratives are retained with their detailed evidence. The current historical register supersedes their blanket closed/fixed claims. The 2026-10-17 triage is a historical snapshot superseded by the October18 review.

## Historical audit narratives

- [hashi-v2-h-series-independent-review-2026-06-12](hashi-v2-h-series-independent-review-2026-06-12.md)
- [hashi-v2-spec-compliance-audit-2026-05-26](hashi-v2-spec-compliance-audit-2026-05-26.md)
- [hashi-v2-spec-compliance-audit-2026-05-28](hashi-v2-spec-compliance-audit-2026-05-28.md)
- [hashi-v2-spec-compliance-audit-2026-06-01](hashi-v2-spec-compliance-audit-2026-06-01.md)
- [hashi-v2-spec-compliance-audit-2026-06-02-e-series](hashi-v2-spec-compliance-audit-2026-06-02-e-series.md)
- [hashi-v2-spec-compliance-audit-2026-06-02](hashi-v2-spec-compliance-audit-2026-06-02.md)
- [hashi-v2-spec-compliance-audit-2026-06-04-f-series](hashi-v2-spec-compliance-audit-2026-06-04-f-series.md)
- [hashi-v2-spec-compliance-audit-2026-06-05-g-series](hashi-v2-spec-compliance-audit-2026-06-05-g-series.md)
- [hashi-v2-spec-compliance-audit-2026-06-09-h-series](hashi-v2-spec-compliance-audit-2026-06-09-h-series.md)

Original normative specifications remain under [implementation-spec](../implementation-spec/README.md); implementation plans and audit suggestions do not override them.
