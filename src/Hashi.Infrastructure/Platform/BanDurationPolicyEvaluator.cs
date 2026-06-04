using System.Text.Json;
using System.Text.Json.Serialization;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Platform;

public sealed record BanDurationPolicy(
    string PolicyType,
    int BaseDurationSeconds,
    decimal LinearMultiplier,
    decimal ExponentialMultiplier,
    int? MaxDurationSeconds,
    int? PermanentAfterCount,
    int CountWindowSeconds,
    int ResetCountAfterSeconds);

public sealed record BanDurationEvaluation(bool IsPermanent, TimeSpan? Duration)
{
    public DateTimeOffset? ExpiresAt(DateTimeOffset now)
        => IsPermanent ? null : now.Add(Duration ?? TimeSpan.Zero);
}

public sealed class BanDurationPolicyEvaluator
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web)
    {
        Converters = { new JsonStringEnumConverter() },
    };

    public BanDurationEvaluation Evaluate(BanDurationPolicy policy, int offenseCount)
    {
        var count = Math.Max(1, offenseCount);
        if (policy.PermanentAfterCount is > 0 && count >= policy.PermanentAfterCount)
        {
            return new BanDurationEvaluation(true, null);
        }

        var seconds = policy.PolicyType.Trim().ToLowerInvariant() switch
        {
            BanDurationPolicyTypeNames.Constant => policy.BaseDurationSeconds,
            BanDurationPolicyTypeNames.Linear => policy.BaseDurationSeconds * (int)Math.Ceiling(Multiplier(policy.LinearMultiplier, count)),
            BanDurationPolicyTypeNames.Exponential => policy.BaseDurationSeconds * Pow(policy.ExponentialMultiplier, count - 1),
            BanDurationPolicyTypeNames.CappedExponential => Math.Min(
                policy.MaxDurationSeconds ?? int.MaxValue,
                policy.BaseDurationSeconds * Pow(policy.ExponentialMultiplier, count - 1)),
            BanDurationPolicyTypeNames.PermanentAfterCount => policy.BaseDurationSeconds,
            _ => throw new InvalidOperationException($"Unsupported ban duration policy type '{policy.PolicyType}'."),
        };

        if (policy.PolicyType.Trim().Equals(BanDurationPolicyTypeNames.Linear, StringComparison.OrdinalIgnoreCase)
            && policy.MaxDurationSeconds is > 0)
        {
            seconds = Math.Min(seconds, policy.MaxDurationSeconds.Value);
        }

        return new BanDurationEvaluation(false, TimeSpan.FromSeconds(Math.Max(0, seconds)));
    }

    public BanDurationPolicy FromJson(string json)
    {
        var contract = JsonSerializer.Deserialize<BanDurationPolicyContract>(json, JsonOptions)
            ?? throw new InvalidOperationException("Ban duration policy JSON is empty.");
        return FromContract(contract);
    }

    public static BanDurationPolicy FromContract(BanDurationPolicyContract contract)
        => new(
            NormalizePolicyType(contract.PolicyType),
            contract.BaseDurationSeconds,
            contract.LinearMultiplier,
            contract.ExponentialMultiplier,
            contract.MaxDurationSeconds,
            contract.PermanentAfterCount,
            contract.CountWindowSeconds,
            contract.ResetCountAfterSeconds);

    private static string NormalizePolicyType(string policyType)
    {
        var normalized = policyType.Trim().ToLowerInvariant();
        return normalized switch
        {
            BanDurationPolicyTypeNames.Constant
                or BanDurationPolicyTypeNames.Linear
                or BanDurationPolicyTypeNames.Exponential
                or BanDurationPolicyTypeNames.CappedExponential
                or BanDurationPolicyTypeNames.PermanentAfterCount => normalized,
            _ => throw new InvalidOperationException($"Unsupported ban duration policy type '{policyType}'."),
        };
    }

    private static decimal Multiplier(decimal configuredMultiplier, int offenseCount)
        => configuredMultiplier <= 0 ? offenseCount : configuredMultiplier * offenseCount;

    private static int Pow(decimal multiplier, int exponent)
    {
        var normalized = multiplier <= 0 ? 2 : multiplier;
        decimal result = 1;
        for (var i = 0; i < exponent; i++)
        {
            result *= normalized;
            if (result > int.MaxValue)
            {
                return int.MaxValue;
            }
        }

        return result > int.MaxValue ? int.MaxValue : (int)Math.Ceiling(result);
    }
}
