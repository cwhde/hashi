# AUD-120: Subject search filters only the most recent two hundred subjects

Priority: P2

Category: Wrong implementation / security search

Status: open; confirmed by manual source trace.

## Finding

SearchAsync takes200 recent subjects before filtering by a nonempty search query in memory. A known older matching IP/CIDR/subject can be omitted even though it exists. The event fallback only considers50 recent matching events, cannot guarantee recovery of older subjects and invokes EnsureSubjectAsync while performing a search. Operators can wrongly conclude that an existing subject or its policy state is absent.

## Evidence

src/Hashi.Infrastructure/Platform/SecuritySubjectOperationsService.cs:17-69. Addendum5.1/12.1 requires reliable lookup of security subjects.

## Expected outcome and acceptance criteria

Apply canonical search predicates before the result limit, use explicit supported search semantics and keep lookup free of state-changing creation. An older subject beyond the first200 must remain discoverable without a recent matching event.
