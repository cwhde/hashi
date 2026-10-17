# Audit records

Use [the current triage](TRIAGE-2026-10-17.md) for what is open and what to do next. It records the reviewed commit, evidence, priority, and verification boundary.

## How to read this folder

- `issues/` keeps the full finding write-ups and their original evidence. Findings are historical records; do not infer that an issue is still open from an old acceptance checklist alone.
- `hashi-v2-spec-compliance-audit-*.md` records successive audit passes and their issue-series scope.
- `hashi-v2-h-series-independent-review-2026-06-12.md` records the independent H-series review and its residual observation.
- Series A through G are in `issues/` as `Axx` through `Gxx`; H-001 through H-100 are in the same directory with `hashi-v2-audit-h-` filenames.

## Status rules

The newest dated triage is the status authority. A finding is closed only when the current code or a later audit explicitly verifies the fix; branch ancestry by itself establishes integration, not correctness. A false positive stays recorded as a false positive. New findings should include a stable ID, category, priority, specification reference, concise impact, evidence, expected behavior, and acceptance criteria. Update the triage and this index when adding an issue series.

## Audit history

| Record | Scope |
| --- | --- |
| [2026-05-26](hashi-v2-spec-compliance-audit-2026-05-26.md) | Initial A-series compliance findings |
| [2026-05-28](hashi-v2-spec-compliance-audit-2026-05-28.md) | B-series follow-up |
| [2026-06-01](hashi-v2-spec-compliance-audit-2026-06-01.md) | C-series findings |
| [2026-06-02](hashi-v2-spec-compliance-audit-2026-06-02.md) | D-series findings |
| [2026-06-02 E-series](hashi-v2-spec-compliance-audit-2026-06-02-e-series.md) | E-series findings |
| [2026-06-04 F-series](hashi-v2-spec-compliance-audit-2026-06-04-f-series.md) | F-series findings |
| [2026-06-05 G-series](hashi-v2-spec-compliance-audit-2026-06-05-g-series.md) | G-series findings |
| [2026-06-09 H-series](hashi-v2-spec-compliance-audit-2026-06-09-h-series.md) | H-001 through H-100 |
| [2026-06-12 independent H review](hashi-v2-h-series-independent-review-2026-06-12.md) | Independent verification and residual legacy dependency note |
| [2026-10-17 current triage](TRIAGE-2026-10-17.md) | Current branch, issue, specification, and verification state |
