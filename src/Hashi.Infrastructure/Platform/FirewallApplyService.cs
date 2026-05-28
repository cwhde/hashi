using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Core.Firewall;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class FirewallApplyService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    SecretRecordService secrets,
    AuditService audit,
    FirewallTrustedIpResolver trustedIpResolver,
    SyncRunService syncRuns)
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
        var allHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        var trustedHosts = await trustedIpResolver.ResolveTrustedPublicIpsAsync(allHosts, cancellationToken);
        var confirmedPorts = await db.TraefikEntryPoints.AsNoTracking()
            .Where(x => x.Confirmed)
            .Select(x => new { x.Port, x.Protocol })
            .ToListAsync(cancellationToken);
        var confirmedKeys = confirmedPorts.Select(x => (x.Port, x.Protocol)).ToHashSet();
        confirmedKeys.Add((80, "tcp"));
        confirmedKeys.Add((443, "tcp"));
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var portForwards = BuildPortForwards(resources, confirmedKeys, host.InternalTraefikIp);

        var overlayCidrs = JsonSerializer.Deserialize<List<string>>(host.NetBirdOverlayCidrsJson) ?? ["100.110.0.0/16"];
        var routedCidrs = JsonSerializer.Deserialize<List<string>>(host.NetBirdRoutedCidrsJson) ?? [];

        return new FirewallHostDefinition(
            host.Id,
            host.Name,
            host.Domain,
            subnets,
            host.LinkedTraefikHost,
            host.InternalTraefikIp,
            host.PublicIp,
            host.WanInterface,
            host.LxcBridge,
            host.NetBirdEnabled,
            host.NetBirdInterface,
            overlayCidrs,
            routedCidrs,
            host.NetBirdRoutingPeer,
            portForwards,
            trustedHosts,
            blocked,
            host.RollbackTimerSeconds);
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

    public async Task<FirewallPlanPreviewResponse> PlanForHostAsync(
        Guid firewallHostId,
        CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleAsync(x => x.Id == firewallHostId, cancellationToken);
        var definition = await BuildHostDefinitionAsync(host, cancellationToken);
        var plan = BuildPlan(host, definition);
        db.FirewallGeneratedScripts.Add(new FirewallGeneratedScriptEntity
        {
            FirewallHostId = host.Id,
            ScriptPath = host.ScriptPath,
            DesiredContentHash = plan.ScriptHash,
            DesiredScript = plan.Preview,
            Status = string.Equals(host.LastAppliedScriptHash, plan.ScriptHash, StringComparison.Ordinal)
                ? FirewallGeneratedScriptStatusNames.Applied
                : FirewallGeneratedScriptStatusNames.Desired,
            AppliedContentHash = host.LastAppliedScriptHash,
            DiffSummary = plan.Changes.SingleOrDefault()?.Summary,
        });
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "firewall",
            "script_planned",
            subjectType: "firewall_host",
            subjectId: host.Id.ToString(),
            metadata: new { plan.PlanId, plan.ScriptHash, changes = plan.Changes.Count },
            cancellationToken: cancellationToken);
        return plan;
    }

    private async Task<FirewallApplyResponse> ApplyDefinitionAsync(
        FirewallHostEntity host,
        FirewallHostDefinition definition,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
    {
        var plan = BuildPlan(host, definition);
        var script = plan.Preview;
        var envFile = FirewallScriptRenderer.RenderEnvFile(definition);
        var scriptHash = plan.ScriptHash;
        var run = await syncRuns.BeginRunAsync("firewall", cancellationToken);
        var generatedScript = new FirewallGeneratedScriptEntity
        {
            FirewallHostId = host.Id,
            SyncRunId = run.Id,
            ScriptPath = host.ScriptPath,
            DesiredContentHash = scriptHash,
            DesiredScript = script,
            Status = FirewallGeneratedScriptStatusNames.Desired,
            AppliedContentHash = host.LastAppliedScriptHash,
            AppliedAtUtc = host.LastAppliedAtUtc,
            DiffSummary = plan.Changes.SingleOrDefault()?.Summary,
        };
        db.FirewallGeneratedScripts.Add(generatedScript);
        await db.SaveChangesAsync(cancellationToken);
        var planChanges = plan.Changes.Select(change => new ProviderChange(
            "firewall-script",
            change.ResourceKey,
            change.Kind == "noop" ? ProviderResultKind.NoOp : ProviderResultKind.Updated,
            change.Summary));
        await syncRuns.AddDiffsAsync(run.Id, planChanges, cancellationToken);
        await audit.WriteAsync(
            "firewall",
            "script_apply_planned",
            subjectType: "sync_run",
            subjectId: run.Id.ToString(),
            metadata: new { hostId = host.Id, plan.PlanId, scriptHash },
            cancellationToken: cancellationToken);

        if (string.Equals(host.LastAppliedScriptHash, scriptHash, StringComparison.Ordinal))
        {
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Applied;
            generatedScript.AppliedContentHash = scriptHash;
            generatedScript.AppliedScript = script;
            generatedScript.AppliedAtUtc = host.LastAppliedAtUtc;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, SyncRiskLevel.None, null, cancellationToken);
            return new FirewallApplyResponse(
                true,
                true,
                host.NetBirdDetected,
                "Script unchanged; skipped apply.",
                plan.PlanId,
                scriptHash,
                plan.Preview);
        }

        var settings = BuildSettings(request);
        var validation = await ValidateConnectivityAsync(settings, request, cancellationToken);
        if (!validation.Succeeded)
        {
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Failed;
            generatedScript.ErrorDetails = validation.Error;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.Medium, validation.Error, cancellationToken);
            return new FirewallApplyResponse(false, false, host.NetBirdDetected, validation.Error, plan.PlanId, scriptHash, plan.Preview);
        }

        var preflight = await RunPreflightAsync(settings, request, cancellationToken);
        if (!preflight.Succeeded)
        {
            var message = preflight.Error ?? preflight.Output;
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Failed;
            generatedScript.ErrorDetails = message;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.Medium, message, cancellationToken);
            return new FirewallApplyResponse(false, false, host.NetBirdDetected, message, plan.PlanId, scriptHash, plan.Preview);
        }

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

        if (!string.IsNullOrWhiteSpace(host.RollbackScript))
        {
            await WriteScriptAsync(
                settings,
                request,
                $"{scriptDir}/hashi-firewall.rollback.sh",
                host.RollbackScript,
                cancellationToken);
        }

        var write = await WriteScriptAsync(settings, request, host.ScriptPath, script, cancellationToken);
        if (!write.Succeeded)
        {
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Failed;
            generatedScript.ErrorDetails = write.Error;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.Medium, write.Error, cancellationToken);
            return new FirewallApplyResponse(false, false, netBird, write.Error, plan.PlanId, scriptHash, plan.Preview);
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

            var message = applyResult.Error ?? applyResult.Output;
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Failed;
            generatedScript.ErrorDetails = message;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, message, cancellationToken);
            return new FirewallApplyResponse(false, false, netBird, message, plan.PlanId, scriptHash, plan.Preview);
        }

        var verification = await VerifyPostApplyAsync(settings, request, cancellationToken);
        if (!verification.Succeeded)
        {
            if (!string.IsNullOrWhiteSpace(host.RollbackScript))
            {
                await RollbackInternalAsync(settings, request, host, cancellationToken);
            }

            var message = verification.Error ?? verification.Output;
            generatedScript.Status = FirewallGeneratedScriptStatusNames.Failed;
            generatedScript.ErrorDetails = message;
            await db.SaveChangesAsync(cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, message, cancellationToken);
            await audit.WriteAsync(
                "firewall",
                "script_apply_failed",
                "failure",
                subjectType: "sync_run",
                subjectId: run.Id.ToString(),
                metadata: new { hostId = host.Id, error = message },
                cancellationToken: cancellationToken);
            return new FirewallApplyResponse(false, false, netBird, message, plan.PlanId, scriptHash, plan.Preview);
        }

        host.LastAppliedScriptHash = scriptHash;
        host.LastAppliedAtUtc = DateTimeOffset.UtcNow;
        generatedScript.Status = FirewallGeneratedScriptStatusNames.Applied;
        generatedScript.AppliedContentHash = scriptHash;
        generatedScript.AppliedScript = script;
        generatedScript.AppliedAtUtc = host.LastAppliedAtUtc;
        await db.SaveChangesAsync(cancellationToken);
        await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, SyncRiskLevel.Low, null, cancellationToken);
        await audit.WriteAsync(
            "firewall",
            "script_applied",
            subjectType: "sync_run",
            subjectId: run.Id.ToString(),
            metadata: new { hostId = host.Id, plan.PlanId, scriptHash },
            cancellationToken: cancellationToken);
        return new FirewallApplyResponse(true, false, netBird, null, plan.PlanId, scriptHash, plan.Preview);
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

        ApplyHostFields(host, request);
        await db.SaveChangesAsync(cancellationToken);
        return host;
    }

    public async Task<FirewallHostEntity?> UpdateHostAsync(Guid id, UpdateFirewallHostRequest request, CancellationToken cancellationToken = default)
    {
        var host = await db.FirewallHosts.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (host is null)
        {
            return null;
        }

        if (request.Name is not null)
        {
            host.Name = request.Name;
        }

        if (request.Domain is not null)
        {
            host.Domain = request.Domain;
        }

        if (request.ManagedSubnets is not null)
        {
            host.ManagedSubnetsJson = JsonSerializer.Serialize(request.ManagedSubnets);
        }

        if (request.LinkedTraefikHost is not null)
        {
            host.LinkedTraefikHost = request.LinkedTraefikHost;
        }

        if (request.InternalTraefikIp is not null)
        {
            host.InternalTraefikIp = request.InternalTraefikIp;
        }

        if (request.ClearPublicIp)
        {
            host.PublicIp = null;
        }
        else if (!string.IsNullOrWhiteSpace(request.PublicIp))
        {
            host.PublicIp = request.PublicIp;
        }

        if (request.ClearWanInterface)
        {
            host.WanInterface = null;
        }
        else if (request.WanInterface is not null)
        {
            host.WanInterface = request.WanInterface;
        }

        if (request.ClearLxcBridge)
        {
            host.LxcBridge = null;
        }
        else if (request.LxcBridge is not null)
        {
            host.LxcBridge = request.LxcBridge;
        }

        if (request.NetBirdEnabled is bool netBirdEnabled)
        {
            host.NetBirdEnabled = netBirdEnabled;
        }

        if (request.NetBirdInterface is not null)
        {
            host.NetBirdInterface = request.NetBirdInterface;
        }

        if (request.NetBirdOverlayCidrs is not null)
        {
            host.NetBirdOverlayCidrsJson = JsonSerializer.Serialize(request.NetBirdOverlayCidrs);
        }

        if (request.NetBirdRoutedCidrs is not null)
        {
            host.NetBirdRoutedCidrsJson = JsonSerializer.Serialize(request.NetBirdRoutedCidrs);
        }

        if (request.NetBirdRoutingPeer is bool routingPeer)
        {
            host.NetBirdRoutingPeer = routingPeer;
        }

        if (request.RollbackTimerSeconds is int rollbackTimer)
        {
            host.RollbackTimerSeconds = rollbackTimer;
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
        host.LastAppliedScriptHash = null;
        host.LastAppliedAtUtc = null;
        await db.SaveChangesAsync(cancellationToken);
        return new FirewallApplyResponse(true, false, host.NetBirdDetected, "Rollback applied.");
    }

    private static void ApplyHostFields(FirewallHostEntity host, CreateFirewallHostRequest request)
    {
        host.Domain = request.Domain;
        host.ManagedSubnetsJson = JsonSerializer.Serialize(request.ManagedSubnets);
        host.LinkedTraefikHost = request.LinkedTraefikHost;
        host.InternalTraefikIp = request.InternalTraefikIp;
        if (!string.IsNullOrWhiteSpace(request.PublicIp))
        {
            host.PublicIp = request.PublicIp;
        }

        if (!string.IsNullOrWhiteSpace(request.WanInterface))
        {
            host.WanInterface = request.WanInterface;
        }

        if (!string.IsNullOrWhiteSpace(request.LxcBridge))
        {
            host.LxcBridge = request.LxcBridge;
        }

        if (request.NetBirdEnabled is bool netBirdEnabled)
        {
            host.NetBirdEnabled = netBirdEnabled;
        }

        if (!string.IsNullOrWhiteSpace(request.NetBirdInterface))
        {
            host.NetBirdInterface = request.NetBirdInterface;
        }

        if (request.NetBirdOverlayCidrs is not null)
        {
            host.NetBirdOverlayCidrsJson = JsonSerializer.Serialize(request.NetBirdOverlayCidrs);
        }

        if (request.NetBirdRoutedCidrs is not null)
        {
            host.NetBirdRoutedCidrsJson = JsonSerializer.Serialize(request.NetBirdRoutedCidrs);
        }

        if (request.NetBirdRoutingPeer is bool routingPeer)
        {
            host.NetBirdRoutingPeer = routingPeer;
        }

        if (request.RollbackTimerSeconds is int rollbackTimer)
        {
            host.RollbackTimerSeconds = rollbackTimer;
        }
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

    private FirewallPlanPreviewResponse BuildPlan(FirewallHostEntity host, FirewallHostDefinition definition)
    {
        var script = FirewallScriptRenderer.Render(definition);
        var scriptHash = ComputeHash(script);
        var hasChanges = !string.Equals(host.LastAppliedScriptHash, scriptHash, StringComparison.Ordinal);
        var changes = new List<FirewallPlanChangeResponse>
        {
            new(
                hasChanges ? "update" : "noop",
                host.Name,
                hasChanges
                    ? $"Render Hashi firewall script {scriptHash} for {host.Name}."
                    : $"Firewall script unchanged at {scriptHash}."),
        };

        return new FirewallPlanPreviewResponse(
            ComputePlanId(host.Id, scriptHash),
            host.Id,
            scriptHash,
            hasChanges,
            changes,
            script);
    }

    private async Task<SshValidationResult> ValidateConnectivityAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
        => request.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(request.Password) =>
                await ssh.ValidateAsync(settings, request.Password, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(request.PrivateKeyPem) =>
                await ssh.ValidateWithPrivateKeyAsync(settings, request.PrivateKeyPem, request.PrivateKeyPassphrase, cancellationToken),
            _ => new SshValidationResult(false, OsFamily.Unknown, null, "Unsupported auth mode."),
        };

    private Task<RemoteCommandResult> RunPreflightAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
    {
        const string command = """
            missing=""
            for cmd in iptables ipset ip sysctl; do
              command -v "$cmd" >/dev/null 2>&1 || missing="$missing $cmd"
            done
            if ! command -v netfilter-persistent >/dev/null 2>&1 && ! command -v systemctl >/dev/null 2>&1 && [ ! -d /etc/cron.d ] && [ ! -w /etc ]; then
              missing="$missing persistence"
            fi
            if [ -n "$missing" ]; then
              echo "Missing required firewall capabilities:$missing" >&2
              exit 2
            fi
            echo "hashi-firewall-preflight-ok"
            """;
        return ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            command,
            cancellationToken);
    }

    private Task<RemoteCommandResult> VerifyPostApplyAsync(
        SshConnectionSettings settings,
        FirewallApplyRequest request,
        CancellationToken cancellationToken)
    {
        const string command = """
            test ! -f /run/hashi-firewall.rollback.pid &&
            iptables -C INPUT -j HASHI_INPUT &&
            iptables -C FORWARD -j HASHI_FWD &&
            iptables -t nat -C PREROUTING -j HASHI_DNAT &&
            iptables -t nat -C POSTROUTING -j HASHI_POSTROUTING
            """;
        return ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            command,
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
            "command -v netbird >/dev/null 2>&1 && netbird status >/dev/null 2>&1 && echo yes || echo no",
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

    private static string ComputeHash(string content)
        => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(content))).ToLowerInvariant();

    private static Guid ComputePlanId(Guid hostId, string scriptHash)
    {
        var input = $"{hostId:N}:{scriptHash}";
        Span<byte> hash = stackalloc byte[32];
        SHA256.HashData(Encoding.UTF8.GetBytes(input), hash);
        return new Guid(hash[..16]);
    }

    private static string Quote(string value) => $"'{value.Replace("'", "'\\''", StringComparison.Ordinal)}'";

    private static IReadOnlyList<FirewallPortForward> BuildPortForwards(
        IReadOnlyList<ResourceEntity> resources,
        HashSet<(int Port, string Protocol)> confirmedKeys,
        string internalTraefikIp)
    {
        var forwards = new List<FirewallPortForward>();
        if (resources.Any(x => x.Kind is "http" or "https" or "h2c"))
        {
            forwards.Add(new FirewallPortForward("tcp", 80, internalTraefikIp, 80));
            forwards.Add(new FirewallPortForward("tcp", 443, internalTraefikIp, 443));
        }

        forwards.AddRange(resources
            .Where(x => x.Kind is "tcp" or "udp")
            .Where(x => confirmedKeys.Contains((x.PublicPort ?? x.TargetPort, x.Kind)))
            .Select(x => new FirewallPortForward(
                x.Kind,
                x.PublicPort ?? x.TargetPort,
                internalTraefikIp,
                x.TargetPort)));

        return forwards
            .GroupBy(x => (Protocol: x.Protocol.ToLowerInvariant(), x.PublicPort, x.TargetHost, x.TargetPort))
            .Select(x => x.First())
            .OrderBy(x => x.Protocol)
            .ThenBy(x => x.PublicPort)
            .ThenBy(x => x.TargetPort)
            .ToList();
    }

    public static FirewallHostResponse ToResponse(FirewallHostEntity host)
    {
        var subnets = JsonSerializer.Deserialize<List<string>>(host.ManagedSubnetsJson) ?? [];
        var overlayCidrs = JsonSerializer.Deserialize<List<string>>(host.NetBirdOverlayCidrsJson) ?? ["100.110.0.0/16"];
        var routedCidrs = JsonSerializer.Deserialize<List<string>>(host.NetBirdRoutedCidrsJson) ?? [];
        return new FirewallHostResponse(
            host.Id,
            host.ConnectionId,
            host.Name,
            host.Domain,
            host.LinkedTraefikHost,
            host.InternalTraefikIp,
            host.PublicIp,
            host.WanInterface,
            subnets,
            host.NetBirdEnabled,
            host.NetBirdInterface,
            overlayCidrs,
            routedCidrs,
            host.NetBirdRoutingPeer,
            host.RollbackTimerSeconds,
            host.NetBirdDetected,
            host.LastAppliedAtUtc);
    }
}
