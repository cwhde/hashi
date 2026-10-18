# AUD-066: Escalating ban durations overflow to zero or fail

Priority: P2

Category: Security / ban duration arithmetic

Status: open; confirmed by manual source tracing.

## Finding

The evaluator multiplies int BaseDurationSeconds by an int factor before applying a cap. Pow saturates at int.MaxValue, but unchecked multiplication still wraps. For base600 and a saturated exponential factor, the product is -600 and Math.Max(0, seconds) yields a zero-length ban. Decimal multiplier overflow/casts can also throw. Stronger repeat offenses can therefore erase enforcement or fail evaluation.

## Evidence

src/Hashi.Infrastructure/Platform/BanDurationPolicyEvaluator.cs:39-57,92-108

## Expected outcome

Use checked or saturating wide arithmetic, apply a validated cap before conversion, and bound policy inputs.

## Acceptance criteria

High offense counts never shorten a ban or throw; capped exponential remains at its cap and uncapped behavior has an explicit safe maximum.

No live provider state was modified to establish this finding.
