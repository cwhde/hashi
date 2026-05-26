using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Core.Firewall;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class FirewallApplyService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    SecretRecordService secrets,
    AuditService audit)
{
    public async Task<IReadOnlyList<FirewallHostResponse>> ListHostsAsync(CancellationToken cancellationToken = default)
    {
        var hosts = await db.FirewallHosts.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        return hosts.Select(ToResponse).ToList();
    }

    public async Task<FirewallApplyResponse> ApplyAsync(FirewallApplyRequest request, CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleOrDefaultAsync(x => x.Id == request.FirewallHostId, cancellationToken)
            ?? throw new InvalidOperationException("Firewall host not found.");

        var definition = await BuildHostDefinitionAsync(host, cancellationToken);
        return await ApplyDefinitionAsync(host, definition, request, cancellationToken);
    }

    public async Task<FirewallApplyResponse> ApplyForHostAsync(Guid firewallHostId, CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleOrDefaultAsync(x => x.Id == firewallHostId, cancellationToken)
            ?? throw new InvalidOperationException("Firewall host not found.");
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == host.ConnectionId, cancellationToken)
            ?? throw new InvalidOperationException("Firewall connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable for firewall host.");

        var definition = await BuildHostDefinitionAsync(host, cancellationToken);
        var request = new FirewallApplyRequest(
            host.Id,
            credentials.Settings.Host,
            credentials.Settings.Port,
            credentials.Settings.Username,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase);
        return await ApplyDefinitionAsync(host, definition, request, cancellationToken);
    }

    public async Task<FirewallHostDefinition> BuildHostDefinitionAsync(
        FirewallHostEntity host,
        CancellationToken cancellationToken = default)
    {
        var subnets = JsonSerializer.Deserialize<List<string>>(host.ManagedSubnetsJson) ?? [];
        var blocked = await db.BlocklistEntries.AsNoTracking()
            .Select(x => x.ClientIp)
            .ToListAsync(cancellationToken);
        var trustedHosts = await db.FirewallHosts.AsNoTracking()
            .Where(x => !string.IsNullOrWhiteSpace(x.PublicIp))
            .Select(x => x.PublicIp!)
            .ToListAsync(cancellationToken);
        var portForwards = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled && (x.Kind == "tcp" || x.Kind == "udp"))
            .Select(x => new FirewallPortForward(x.Kind, x.TargetPort, host.InternalTraefikIp, x.TargetPort))
            .ToListAsync(cancellationToken);

        return new FirewallHostDefinition(
            host.Id,
            host.Name,
            host.Domain,
            subnets,
            host.LinkedTraefikHost,
            host.InternalTraefikIp,
            host.PublicIp,
            PortForwards: portForwards,
            TrustedPublicIps: trustedHosts,
            BlockedIps: blocked);
    }

    public async Task<(string Script, string ScriptHash)> RenderForHostAsync(
        Guid firewallHostId,
        CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleAsync(x => x.Id == firewallHostId, cancellationToken);
        var definition = await BuildHostDefinitionAsync(host, cancellationToken);
        var script = FirewallScriptRenderer.Render(definition);
        var hash = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(script))).ToLowerInvariant();
        return (script, hash);
    }

    private async Task<FirewallApplyResponse> ApplyDefinitionAsync(
        FirewallHostEntity host,
        FirewallHostDefinition definition,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
    {
        var script = FirewallScriptRenderer.Render(definition);
        var envFile = FirewallScriptRenderer.RenderEnvFile(definition);
        var scriptHash = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(script))).ToLowerInvariant();

        if (string.Equals(host.LastAppliedScriptHash, scriptHash, StringComparison.Ordinal))
        {
            return new FirewallApplyResponse(true, true, host.NetBirdDetected, "Script unchanged; skipped apply.");
        }

        var settings = BuildSettings(request);
        var netBird = await DetectNetBirdAsync(settings, request, cancellationToken);
        host.NetBirdDetected = netBird;

        if (!string.IsNullOrWhiteSpace(host.LastAppliedScriptHash))
        {
            var previous = await ssh.ReadFileAsync(
                settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
                host.ScriptPath, cancellationToken);
            if (previous.Succeeded && previous.Content is not null)
            {
                host.RollbackScript = Encoding.UTF8.GetString(previous.Content);
            }
        }

        var scriptDir = "/opt/hashi/firewall";
        await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            $"mkdir -p {Quote(scriptDir)}",
            cancellationToken);

        var write = await WriteScriptAsync(settings, request, host.ScriptPath, script, cancellationToken);
        if (!write.Succeeded)
        {
            return new FirewallApplyResponse(false, false, netBird, write.Error);
        }

        await WriteScriptAsync(settings, request, $"{scriptDir}/hashi-firewall.env", envFile, cancellationToken);

        var applyResult = await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            $"chmod +x {Quote(host.ScriptPath)} && {Quote(host.ScriptPath)}",
            cancellationToken);
        if (!applyResult.Succeeded)
        {
            if (!string.IsNullOrWhiteSpace(host.RollbackScript))
            {
                await RollbackInternalAsync(settings, request, host, cancellationToken);
            }

            return new FirewallApplyResponse(false, false, netBird, applyResult.Error);
        }

        host.LastAppliedScriptHash = scriptHash;
        host.LastAppliedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("firewall", "script_applied", subjectType: "firewall_host", subjectId: host.Id.ToString(), cancellationToken: cancellationToken);
        return new FirewallApplyResponse(true, false, netBird, null);
    }

    public async Task<FirewallHostEntity> UpsertHostAsync(CreateFirewallHostRequest request, CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleOrDefaultAsync(
            x => x.ConnectionId == request.ConnectionId && x.Name == request.Name,
            cancellationToken);
        if (host is null)
        {
            host = new FirewallHostEntity
            {
                ConnectionId = request.ConnectionId,
                Name = request.Name,
            };
            db.FirewallHosts.Add(host);
        }

        host.Domain = request.Domain;
        host.ManagedSubnetsJson = JsonSerializer.Serialize(request.ManagedSubnets);
        host.LinkedTraefikHost = request.LinkedTraefikHost;
        host.InternalTraefikIp = request.InternalTraefikIp;
        if (!string.IsNullOrWhiteSpace(request.PublicIp))
        {
            host.PublicIp = request.PublicIp;
        }

        await db.SaveChangesAsync(cancellationToken);
        return host;
    }

    public async Task<FirewallApplyResponse> RollbackAsync(Guid firewallHostId, FirewallApplyRequest request, CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleAsync(x => x.Id == firewallHostId, cancellationToken);
        if (string.IsNullOrWhiteSpace(host.RollbackScript))
        {
            return new FirewallApplyResponse(false, false, host.NetBirdDetected, "No rollback script stored.");
        }

        var settings = BuildSettings(request);
        await RollbackInternalAsync(settings, request, host, cancellationToken);
        return new FirewallApplyResponse(true, false, host.NetBirdDetected, "Rollback applied.");
    }

    private async Task RollbackInternalAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        FirewallHostEntity host,
        CancellationToken cancellationToken)
    {
        var rollback = host.RollbackScript ?? string.Empty;
        await WriteScriptAsync(settings, request, host.ScriptPath, rollback, cancellationToken);
        await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            Quote(host.ScriptPath),
            cancellationToken);
    }

    private async Task<RemoteWriteResult> WriteScriptAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        string remotePath,
        string script,
        CancellationToken cancellationToken)
        => request.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(request.Password) =>
                await ssh.WriteAtomicAsync(settings, request.Password, remotePath, Encoding.UTF8.GetBytes(script), cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(request.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    settings, request.PrivateKeyPem, request.PrivateKeyPassphrase, remotePath, Encoding.UTF8.GetBytes(script), cancellationToken),
            _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };

    private async Task<bool> DetectNetBirdAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
    {
        var result = await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            "command -v netbird >/dev/null 2>&1 && echo yes || echo no",
            cancellationToken);
        return result.Succeeded && result.Output.Contains("yes", StringComparison.OrdinalIgnoreCase);
    }

    private static SshConnectionSettings BuildSettings(FirewallApplyRequest request) => new(
        request.Host,
        request.Port <= 0 ? 22 : request.Port,
        request.Username,
        OsFamily.Unknown,
        null,
        null);

    private static string Quote(string value) => $"'{value.Replace("'", "'\\''", StringComparison.Ordinal)}'";

    internal static FirewallHostResponse ToResponse(FirewallHostEntity host)
    {
        var subnets = JsonSerializer.Deserialize<List<string>>(host.ManagedSubnetsJson) ?? [];
        return new FirewallHostResponse(
            host.Id,
            host.ConnectionId,
            host.Name,
            host.Domain,
            host.LinkedTraefikHost,
            host.InternalTraefikIp,
            host.PublicIp,
            subnets,
            host.NetBirdDetected,
            host.LastAppliedAtUtc);
    }
}
