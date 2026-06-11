using Hashi.Infrastructure.Crypto;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Auth;

public sealed class ServiceSyncVaultBootstrapper(
    ServiceSyncVaultState serviceSync,
    ILogger<ServiceSyncVaultBootstrapper> logger) : IHostedService
{
    public Task StartAsync(CancellationToken cancellationToken)
    {
        var configured = Environment.GetEnvironmentVariable("HASHI_SERVICE_SYNC_VAULT_KEY");

        if (string.IsNullOrWhiteSpace(configured))
        {
            logger.LogWarning("Service-sync vault key is not configured; background secret sync will pause until configured.");
            return Task.CompletedTask;
        }

        try
        {
            var secretBytes = Convert.FromBase64String(configured.Trim());
            if (secretBytes.Length < 32)
            {
                logger.LogError("Service-sync vault key must be at least 32 bytes when base64-decoded.");
                return Task.CompletedTask;
            }

            var wrapKey = KeyDerivation.DeriveServiceSyncWrapKey(secretBytes);
            serviceSync.Initialize(wrapKey);
            logger.LogInformation("Service-sync vault key loaded from the process environment.");
        }
        catch (FormatException ex)
        {
            logger.LogError(ex, "Service-sync vault key is not valid base64.");
        }

        return Task.CompletedTask;
    }

    public Task StopAsync(CancellationToken cancellationToken) => Task.CompletedTask;
}
