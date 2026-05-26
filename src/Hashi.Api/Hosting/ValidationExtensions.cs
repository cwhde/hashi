using FluentValidation;
using FluentValidation.Results;

namespace Hashi.Api.Hosting;

public static class ValidationExtensions
{
    public static async Task<Dictionary<string, string[]>?> ValidateRequestAsync<T>(
        this IValidator<T> validator,
        T request,
        CancellationToken cancellationToken = default)
    {
        var validationResult = await validator.ValidateAsync(request, cancellationToken);
        return validationResult.IsValid ? null : ToValidationErrors(validationResult);
    }

    private static Dictionary<string, string[]> ToValidationErrors(ValidationResult result)
        => result.Errors
            .GroupBy(
                x => string.IsNullOrWhiteSpace(x.PropertyName) ? "request" : x.PropertyName,
                StringComparer.OrdinalIgnoreCase)
            .ToDictionary(
                x => x.Key,
                x => x.Select(error => error.ErrorMessage).Distinct(StringComparer.Ordinal).ToArray(),
                StringComparer.OrdinalIgnoreCase);
}