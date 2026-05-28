using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class CertificateSetupService(
    HashiDbContext db,
    AppSettingsService settings,
    SecretRecordService secrets,
    VaultSessionState vault,
    AuditService audit)
{
    public async Task<CertificateSetupResponse> GetAsync(CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var (keyId, _) = await ResolveEabCredentialsAsync(appSettings, cancellationToken);
        var resolvers = ParseResolvers(appSettings.AcmeResolversJson);
        return new CertificateSetupResponse(
            appSettings.AcmeEmail,
            !string.IsNullOrWhiteSpace(keyId),
            appSettings.DnsChallengeDelaySeconds,
            resolvers,
            HasDnsProvider: await db.Connections.AsNoTracking().AnyAsync(x => x.Type == "dns" && x.Enabled, cancellationToken));
    }

    public async Task<CertificateSetupValidateResponse> ValidateAsync(
        CertificateSetupRequest request,
        CancellationToken cancellationToken = default)
    {
        var errors = new List<string>();
        if (string.IsNullOrWhiteSpace(request.AcmeEmail) || !request.AcmeEmail.Contains('@', StringComparison.Ordinal))
        {
            errors.Add("ACME email is required.");
        }

        if (string.IsNullOrWhiteSpace(request.EabKeyId) || string.IsNullOrWhiteSpace(request.EabHmac))
        {
            errors.Add("EAB key ID and HMAC are required for Google Trust Services.");
        }

        var hasDns = await db.Connections.AsNoTracking().AnyAsync(x => x.Type == "dns" && x.Enabled, cancellationToken);
        if (!hasDns)
        {
            errors.Add("Configure an enabled DNS provider connection before ACME setup.");
        }

        if (request.DnsChallengeDelaySeconds is < 0 or > 600)
        {
            errors.Add("DNS challenge delay must be between 0 and 600 seconds.");
        }

        foreach (var resolver in request.Resolvers ?? [])
        {
            if (string.IsNullOrWhiteSpace(resolver))
            {
                errors.Add("Resolver list cannot contain empty entries.");
            }
        }

        return new CertificateSetupValidateResponse(errors.Count == 0, errors);
    }

    public async Task<CertificateSetupSaveResponse> SaveAsync(
        CertificateSetupRequest request,
        CancellationToken cancellationToken = default)
    {
        var validation = await ValidateAsync(request, cancellationToken);
        if (!validation.IsValid)
        {
            return new CertificateSetupSaveResponse(false, string.Join(' ', validation.Errors));
        }

        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        appSettings.AcmeEmail = request.AcmeEmail.Trim();
        appSettings.DnsChallengeDelaySeconds = request.DnsChallengeDelaySeconds;
        appSettings.AcmeResolversJson = JsonSerializer.Serialize(request.Resolvers ?? ["1.1.1.1:53", "8.8.8.8:53"]);
        appSettings.UpdatedAtUtc = DateTimeOffset.UtcNow;

        if (!vault.IsUnlocked)
        {
            await db.SaveChangesAsync(cancellationToken);
            await audit.WriteAsync(
                "setup",
                "certificate_provider_saved_without_eab",
                subjectType: "setup",
                outcome: "failed",
                cancellationToken: cancellationToken);
            return new CertificateSetupSaveResponse(false, "Unlock the vault before saving ACME EAB credentials.");
        }

        var eabPayload = JsonSerializer.Serialize(new AcmeEabPayload(request.EabKeyId.Trim(), request.EabHmac.Trim()));
        var stored = await secrets.StoreAsync(
            SecretPurpose.AcmeEab,
            "ACME EAB",
            Encoding.UTF8.GetBytes(eabPayload),
            cancellationToken,
            serviceSyncEligible: true);
        appSettings.AcmeEabSecretId = stored.Id;

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("setup", "certificate_provider_saved", subjectType: "setup", cancellationToken: cancellationToken);
        return new CertificateSetupSaveResponse(true, null);
    }

    public Task MigratePendingEabToVaultAsync(CancellationToken cancellationToken = default) => Task.CompletedTask;

    public async Task<TraefikRenderOptions> BuildTraefikOptionsAsync(
        string adminDomain,
        CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var (keyId, hmac) = await ResolveEabCredentialsAsync(appSettings, cancellationToken);
        return new TraefikRenderOptions(
            AcmeEmail: appSettings.AcmeEmail,
            AcmeEabKeyId: keyId,
            AcmeEabHmac: hmac,
            DnsChallengeDelaySeconds: appSettings.DnsChallengeDelaySeconds,
            AcmeResolvers: ParseResolvers(appSettings.AcmeResolversJson),
            AdminDomain: adminDomain);
    }

    private async Task<(string? KeyId, string? Hmac)> ResolveEabCredentialsAsync(
        Persistence.Entities.AppSettingsEntity appSettings,
        CancellationToken cancellationToken)
    {
        if (appSettings.AcmeEabSecretId is Guid secretId)
        {
            var plaintext = await secrets.DecryptForPurposeAsync(secretId, cancellationToken);
            if (plaintext is not null)
            {
                return ParseEabPayload(plaintext);
            }
        }

        return (null, null);
    }

    private static (string? KeyId, string? Hmac) ParseEabPayload(byte[] payload)
    {
        try
        {
            var parsed = JsonSerializer.Deserialize<AcmeEabPayload>(payload);
            return parsed is null ? (null, null) : (parsed.KeyId, parsed.Hmac);
        }
        catch (JsonException)
        {
            return (null, null);
        }
    }

    private static IReadOnlyList<string> ParseResolvers(string json)
    {
        try
        {
            return JsonSerializer.Deserialize<List<string>>(json) ?? ["1.1.1.1:53", "8.8.8.8:53"];
        }
        catch (JsonException)
        {
            return ["1.1.1.1:53", "8.8.8.8:53"];
        }
    }

    private sealed record AcmeEabPayload(string KeyId, string Hmac);
}
