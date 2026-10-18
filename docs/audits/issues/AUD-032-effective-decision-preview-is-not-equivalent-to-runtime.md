# AUD-032: Effective-decision preview does not reproduce runtime policy evaluation

Priority: P2 — incident-response view can report a misleading decision

Category: Wrong feature behavior / security observability

Spec reference: implementation addendum §5.1 requires the security view to show each subject's current effective decision and matching policy context; §5.2 defines deterministic decision precedence.

## Finding

The subject detail endpoint does not consistently evaluate the runtime decision. For IP subjects it calls the runtime engine with the synthetic host `subject-preview.hashi.local`, so resource-scoped manual entries and rules for the actual host are not evaluated. For non-IP subjects it uses a separate heuristic that marks any enabled matching blocklist entry as a deny without checking enforcement mode, manual-allow bypass flags, or the runtime precedence. The UI can therefore show an allow/deny result that differs from an actual request, which weakens the incident-response view specified for this feature.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecuritySubjectOperationsService.cs`: `GetEffectiveDecisionAsync` passes `subject-preview.hashi.local` and `/` to `DecideForwardAuthAsync` for IP subjects, with no resource or request-context selection.
- The same method routes non-IP subjects to `ExplainNonIpDecisionAsync` rather than the runtime decision engine.
- `ExplainNonIpDecisionAsync` derives deny from any enabled manual block or any enabled matching blocklist row; it does not filter observe-only blocklists or apply runtime manual-allow bypass and precedence behavior.
- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`: runtime evaluation resolves resource from the requested host and filters blocklists by enforcement mode, making the two preview paths observably different.

## Expected outcome

Make the preview's context explicit and evaluate the same policy engine used at runtime. If a subject can have different decisions on different resources, present per-resource decisions or require the operator to choose the host/resource. For non-IP subjects, use matching semantics and precedence equivalent to runtime evaluation, or label the result as partial rather than “effective”.

## Acceptance criteria

- A resource-scoped manual entry or resource rule is reflected for the selected resource and excluded for unrelated resources.
- Observe-only blocklist matches do not appear as enforced denials.
- Manual-allow bypass flags and runtime decision precedence produce the same result in preview and enforcement for equivalent request context.
- ASN/country/region previews identify when missing request context prevents a definitive decision.
- The response/UI distinguishes an exact decision from a context-dependent or partial explanation.
