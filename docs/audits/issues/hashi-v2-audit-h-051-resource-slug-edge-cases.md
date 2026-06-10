# H-051: ResourceModels ResourceSlug Does Not Handle Empty Or Invalid Input Properly

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §6, Addendum §10.3

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`ResourceModels.ResourceSlug.Normalize()` in `src/Hashi.Core/Resources/ResourceModels.cs` handles edge cases incorrectly:

1. **Empty input returns empty string**: `Normalize("")` returns `""`, which would cause downstream issues (resources with no slug, route matching failures).
2. **Consecutive non-alphanumeric chars not collapsed**: `Normalize("hello--world")` produces `"hello--world"` instead of `"hello-world"`.
3. **No length limit**: Extremely long names produce extremely long slugs with no truncation.

The addendum §10.3 specifies similar normalization rules for Pulse agent DNS names: "Replace spaces/invalid chars with hyphens. Collapse repeated hyphens. Trim leading/trailing hyphens. Reject empty result." These same rules should apply to resource slugs for consistency.

## Evidence

```csharp
// ResourceModels.cs — ResourceSlug.Normalize likely has:
// - No empty string guard
// - No consecutive dash collapsing
// - No length truncation
```

## Expected Outcome

`Normalize` should:
1. Return an error or generate a fallback slug for empty input (e.g., based on a GUID)
2. Collapse consecutive non-alphanumeric characters into a single hyphen
3. Trim leading/trailing hyphens
4. Enforce a maximum length (e.g., 63 characters for DNS label compatibility)
5. Handle Unicode properly (transliterate or reject)

## Fix Guidance

1. Add empty input validation: throw `ArgumentException` or generate a GUID-based fallback.
2. After replacing non-alphanumeric chars with hyphens, collapse consecutive hyphens with a regex: `Regex.Replace(result, @"-+", "-")`.
3. Add `.Trim('-')`.
4. Enforce max length: `result.Length > 63 ? result[..63].TrimEnd('-') : result`.
5. Add unit tests for empty, multiple-hyphens, leading/trailing hyphens, unicode, and length limits.

## Acceptance Criteria

- [ ] Empty input produces a valid non-empty slug or throws a clear error
- [ ] Multiple consecutive non-alphanumeric characters collapse to a single hyphen
- [ ] Leading and trailing hyphens are trimmed
- [ ] Slug does not exceed maximum length (63 chars for DNS compatibility)
- [ ] Unit tests cover all edge cases
