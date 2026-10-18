# AUD-123: Session and composite manual entries are accepted but never match live requests

Priority: P2

Category: Missing implementation / security subject types

Status: open; confirmed by manual source trace.

## Finding

SecuritySubjectNormalizer accepts Session and Composite opaque subjects, so manual entry creation can persist them. Its Matches function handles only IP/CIDR/country/region/ASN and returns false for these types. Live request/context matching supplies no session/composite identity. Such an entry can appear configured in the subject UI yet never allow or block the intended traffic.

## Evidence

src/Hashi.Infrastructure/Platform/SecuritySubjectNormalizer.cs:19-30,45-65; SecurityDecisionService.cs:575-595; SecuritySubjectOperationsService.cs manual create/update normalization. Addendum subject model includes these identities.

## Expected outcome and acceptance criteria

Implement explicit canonical request identities and matching for supported session/composite policies, or reject unsupported types clearly until implemented. Preserve opaque identity case when relevant and demonstrate an actual matching and nonmatching request for each accepted type.
