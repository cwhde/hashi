using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class CertificateSetupValidationException(string message) : InvalidOperationException(message);

public sealed record TraefikAcmeDnsCredential(
    string ProviderName,
    string EnvironmentVariable,
    string Token);

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
            HasDnsProvider: await db.Connections.AsNoTracking().AnyAsync(
                x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled,
                cancellationToken),
            appSettings.AcmeDnsProviderConnectionId);
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

        await ValidateDnsProviderBindingAsync(
            request.DnsProviderConnectionId,
            requireServiceSyncCredential: true,
            errors,
            cancellationToken);

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
        appSettings.AcmeDnsProviderConnectionId = request.DnsProviderConnectionId;
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
        var provider = string.IsNullOrWhiteSpace(appSettings.AcmeEmail)
            ? null
            : await ResolveDnsProviderBindingOrThrowAsync(
                appSettings.AcmeDnsProviderConnectionId,
                requireServiceSyncCredential: false,
                cancellationToken);
        return new TraefikRenderOptions(
            AcmeEmail: appSettings.AcmeEmail,
            AcmeEabKeyId: keyId,
            AcmeEabHmac: hmac,
            DnsProviderName: provider?.ProviderName,
            AcmeProvider: appSettings.AcmeProvider,
            DnsChallengeDelaySeconds: appSettings.DnsChallengeDelaySeconds,
            AcmeResolvers: ParseResolvers(appSettings.AcmeResolversJson),
            AdminDomain: adminDomain);
    }

    public async Task<TraefikAcmeDnsCredential?> BuildTraefikAcmeDnsCredentialAsync(
        CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        if (string.IsNullOrWhiteSpace(appSettings.AcmeEmail))
        {
            return null;
        }

        var provider = await ResolveDnsProviderBindingOrThrowAsync(
            appSettings.AcmeDnsProviderConnectionId,
            requireServiceSyncCredential: true,
            cancellationToken);
        return new TraefikAcmeDnsCredential(
            provider.ProviderName,
            provider.EnvironmentVariable,
            provider.Token ?? throw new CertificateSetupValidationException("Selected DNS provider credentials are unavailable to service sync."));
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

    private async Task<DnsProviderBinding> ResolveDnsProviderBindingOrThrowAsync(
        Guid? connectionId,
        bool requireServiceSyncCredential,
        CancellationToken cancellationToken)
    {
        var errors = new List<string>();
        var binding = await ValidateDnsProviderBindingAsync(
            connectionId,
            requireServiceSyncCredential,
            errors,
            cancellationToken);
        if (errors.Count > 0 || binding is null)
        {
            throw new CertificateSetupValidationException(string.Join(' ', errors));
        }

        return binding;
    }

    private async Task<DnsProviderBinding?> ValidateDnsProviderBindingAsync(
        Guid? connectionId,
        bool requireServiceSyncCredential,
        List<string> errors,
        CancellationToken cancellationToken)
    {
        if (connectionId is null)
        {
            errors.Add("Select an enabled DNS provider connection for ACME DNS challenge.");
            return null;
        }

        var connection = await db.Connections.AsNoTracking()
            .SingleOrDefaultAsync(x => x.Id == connectionId.Value, cancellationToken);
        if (connection is null || connection.Type != ConnectionTypeNames.DnsProvider)
        {
            errors.Add("Selected DNS provider connection was not found.");
            return null;
        }

        if (!connection.Enabled)
        {
            errors.Add("Selected DNS provider connection is disabled.");
            return null;
        }

        var provider = ParseDnsProviderType(connection.SettingsJson);
        if (string.IsNullOrWhiteSpace(provider))
        {
            errors.Add("Selected DNS provider connection has no provider type configured.");
            return null;
        }

        if (!TryMapTraefikDnsProvider(provider, out var traefikProvider, out var envVar))
        {
            errors.Add($"Selected DNS provider '{provider}' is not supported for Traefik ACME DNS challenge.");
            return null;
        }

        if (connection.SecretId is null)
        {
            errors.Add("Selected DNS provider connection has no stored credentials.");
            return null;
        }

        string? token = null;
        if (requireServiceSyncCredential)
        {
            var tokenBytes = await secrets.DecryptForServiceSyncAsync(connection.SecretId.Value, cancellationToken);
            if (tokenBytes is null)
            {
                errors.Add("Selected DNS provider credentials are unavailable to service sync; configure the service-sync vault and re-save the DNS provider connection.");
                return null;
            }

            token = Encoding.UTF8.GetString(tokenBytes);
            if (string.IsNullOrWhiteSpace(token))
            {
                errors.Add("Selected DNS provider credential is empty.");
                return null;
            }
        }

        return new DnsProviderBinding(traefikProvider, envVar, token);
    }

    private static string? ParseDnsProviderType(string settingsJson)
    {
        try
        {
            return JsonSerializer.Deserialize<DnsConnectionSettings>(settingsJson)?.Provider;
        }
        catch (JsonException)
        {
            return null;
        }
    }

    private static bool TryMapTraefikDnsProvider(
        string provider,
        out string traefikProvider,
        out string environmentVariable)
    {
        if (string.Equals(provider, DnsProviderTypeNames.Hetzner, StringComparison.OrdinalIgnoreCase))
        {
            traefikProvider = DnsProviderTypeNames.Hetzner;
            environmentVariable = "HETZNER_API_KEY";
            return true;
        }

        traefikProvider = string.Empty;
        environmentVariable = string.Empty;
        return false;
    }

    private sealed record AcmeEabPayload(string KeyId, string Hmac);

    private sealed record DnsProviderBinding(
        string ProviderName,
        string EnvironmentVariable,
        string? Token);

    private sealed record DnsConnectionSettings(
        [property: JsonPropertyName("provider")] string Provider,
        [property: JsonPropertyName("zoneName")] string? ZoneName = null,
        [property: JsonPropertyName("defaultTtl")] int? DefaultTtl = null);
}
