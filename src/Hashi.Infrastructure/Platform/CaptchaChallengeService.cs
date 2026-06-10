using System.Net;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public interface ICapClient
{
    Task<CapVerifyResult> VerifyAsync(
        Uri capInstanceBaseUrl,
        string siteKey,
        string secretKey,
        string token,
        TimeSpan timeout,
        CancellationToken cancellationToken = default);
}

public sealed record CapVerifyResult(string Status, bool Success, string? Error = null)
{
    public static CapVerifyResult Verified() => new("verified", true);

    public static CapVerifyResult Failed(string? error = null) => new("failed", false, error);

    public static CapVerifyResult Unavailable(string? error = null) => new("unavailable", false, error);
}

public sealed class CapClient(IHttpClientFactory httpClientFactory, ILogger<CapClient> logger) : ICapClient
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    public async Task<CapVerifyResult> VerifyAsync(
        Uri capInstanceBaseUrl,
        string siteKey,
        string secretKey,
        string token,
        TimeSpan timeout,
        CancellationToken cancellationToken = default)
    {
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        cts.CancelAfter(timeout);

        try
        {
            var endpoint = BuildSiteVerifyEndpoint(capInstanceBaseUrl, siteKey);
            using var request = new HttpRequestMessage(HttpMethod.Post, endpoint)
            {
                Content = JsonContent.Create(new CapSiteVerifyRequest(secretKey, token), options: JsonOptions),
            };

            var client = httpClientFactory.CreateClient("cap");
            using var response = await client.SendAsync(request, cts.Token);
            if (!response.IsSuccessStatusCode)
            {
                logger.LogWarning("Cap siteverify returned HTTP {StatusCode}.", (int)response.StatusCode);
                return response.StatusCode == HttpStatusCode.BadRequest
                    || response.StatusCode == HttpStatusCode.Unauthorized
                    || response.StatusCode == HttpStatusCode.Forbidden
                        ? CapVerifyResult.Failed("Cap rejected the token.")
                        : CapVerifyResult.Unavailable("Cap verification is unavailable.");
            }

            var body = await response.Content.ReadFromJsonAsync<CapSiteVerifyResponse>(JsonOptions, cts.Token);
            return body?.Success == true
                ? CapVerifyResult.Verified()
                : CapVerifyResult.Failed("Cap token verification failed.");
        }
        catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
        {
            return CapVerifyResult.Unavailable("Cap verification timed out.");
        }
        catch (Exception ex) when (ex is HttpRequestException or JsonException or UriFormatException)
        {
            logger.LogWarning(ex, "Cap siteverify failed without exposing secret material.");
            return CapVerifyResult.Unavailable("Cap verification is unavailable.");
        }
    }

    private static Uri BuildSiteVerifyEndpoint(Uri capInstanceBaseUrl, string siteKey)
    {
        var baseText = capInstanceBaseUrl.ToString().TrimEnd('/');
        var siteSegment = Uri.EscapeDataString(siteKey.Trim());
        if (!baseText.EndsWith("/" + siteSegment, StringComparison.OrdinalIgnoreCase))
        {
            baseText = $"{baseText}/{siteSegment}";
        }

        return new Uri($"{baseText}/siteverify", UriKind.Absolute);
    }

    private sealed record CapSiteVerifyRequest(string Secret, string Response);

    private sealed record CapSiteVerifyResponse(bool Success);
}

public sealed class CaptchaChallengeService(
    HashiDbContext db,
    SecretRecordService secrets,
    ICapClient capClient,
    AuditService audit,
    BanDurationPolicyEvaluator banPolicies,
    TimeProvider? timeProvider = null)
{
    public const string PublicChallengeSystemKey = "captcha_public_challenge";
    public const string PublicChallengeWorkflow = "captcha";

    public async Task<CaptchaSettingsResponse> GetSettingsAsync(CancellationToken cancellationToken = default)
        => ToResponse(await GetOrCreateSettingsAsync(cancellationToken));

    public async Task<CaptchaSettingsResponse> UpdateSettingsAsync(
        CaptchaSettingsRequest request,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        ValidateSettingsRequest(request, settings);

        settings.Enabled = request.Enabled;
        settings.PublicChallengeBaseUrl = NormalizeNullableUrl(request.PublicChallengeBaseUrl);
        settings.SiteKey = NormalizeNullable(request.SiteKey);
        settings.VerificationTimeoutSeconds = Math.Clamp(request.VerificationTimeoutSeconds ?? settings.VerificationTimeoutSeconds, 1, 30);
        settings.InstrumentationExpected = request.InstrumentationExpected ?? settings.InstrumentationExpected;
        settings.HeadlessDetectionExpected = request.HeadlessDetectionExpected ?? settings.HeadlessDetectionExpected;
        settings.CapAdminResourceId = request.CapAdminResourceId;
        settings.CapAdminDomain = NormalizeDomain(request.CapAdminDomain);
        settings.PublicChallengeResourceId = request.PublicChallengeResourceId ?? settings.PublicChallengeResourceId;
        settings.PublicChallengeDomain = NormalizeDomain(request.PublicChallengeDomain);
        settings.ChallengeResetMode = NormalizeChallengeResetMode(request.ChallengeResetMode ?? settings.ChallengeResetMode);
        settings.ChallengeDecayPercent = Math.Clamp(request.ChallengeDecayPercent ?? settings.ChallengeDecayPercent, 0, 100);
        settings.MinimumRepeatChallengeSeconds = Math.Clamp(request.MinimumRepeatChallengeSeconds ?? settings.MinimumRepeatChallengeSeconds, 0, 86400);
        settings.MaximumFailuresBeforeEscalation = Math.Clamp(request.MaximumFailuresBeforeEscalation ?? settings.MaximumFailuresBeforeEscalation, 1, 100);
        settings.MaximumRequestsWhileChallenged = Math.Clamp(request.MaximumRequestsWhileChallenged ?? settings.MaximumRequestsWhileChallenged, 1, 10000);

        if (!string.IsNullOrWhiteSpace(request.SecretKey))
        {
            var stored = await secrets.StoreAsync(
                SecretPurpose.CapSecretKey,
                "Cap siteverify key secret",
                Encoding.UTF8.GetBytes(request.SecretKey),
                cancellationToken,
                serviceSyncEligible: true);
            settings.SecretKeySecretId = stored.Id;
        }
        else if (request.SecretKeySecretId is Guid secretId)
        {
            settings.SecretKeySecretId = secretId;
        }

        if (settings.Enabled)
        {
            await EnsurePublicChallengeResourceAsync(settings, cancellationToken);
            await EnsureOptionalAdminResourceAsync(settings, cancellationToken);
        }
        else
        {
            await ReleasePublicChallengeResourceAsync(settings, cancellationToken);
        }

        settings.UpdatedAtUtc = Now();
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "security",
            "captcha_settings_updated",
            subjectType: "captcha_settings",
            subjectId: settings.Id.ToString(),
            metadata: new { settings.Enabled, settings.PublicChallengeDomain, settings.CapAdminDomain },
            cancellationToken: cancellationToken);

        return ToResponse(settings);
    }

    public async Task<CaptchaTestResponse> TestAsync(
        CaptchaTestRequest request,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        var result = await VerifyTokenWithCapAsync(settings, request.Token, cancellationToken);
        return new CaptchaTestResponse(result.Success, result.Status, result.Error);
    }

    public async Task<CaptchaChallengeStatusResponse> GetChallengeStatusAsync(
        IPAddress clientIp,
        string? returnUrl,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        if (!settings.Enabled)
        {
            return new CaptchaChallengeStatusResponse(false, false, null, null, null, returnUrl, "/");
        }

        var (_, state) = await new SecuritySubjectService(db, timeProvider)
            .ResolveOrCreateIpAsync(clientIp, null, null, null, cancellationToken);

        return new CaptchaChallengeStatusResponse(
            true,
            state.ChallengeRequired,
            state.ChallengeReason,
            settings.SiteKey,
            BuildCapWidgetEndpoint(settings),
            returnUrl,
            await SafeReturnUrlAsync(returnUrl, cancellationToken));
    }

    public async Task RecordChallengePageRequestAsync(
        IPAddress clientIp,
        string? returnUrl,
        CancellationToken cancellationToken = default)
    {
        var normalized = SecuritySubjectNormalizer.NormalizeIp(clientIp);
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "captcha",
            Action = "challenge_page_requested",
            ClientIp = normalized.NormalizedValue,
            SubjectType = normalized.SubjectType,
            SubjectValue = normalized.SubjectValue,
            NormalizedSubjectValue = normalized.NormalizedValue,
            EventType = "captcha_page_requested",
            Decision = "challenge",
            Source = "hashi_edge_challenge",
            MetadataJson = JsonSerializer.Serialize(new { returnUrl }),
            OccurredAtUtc = Now(),
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<CaptchaChallengeVerifyResponse> VerifyChallengeAsync(
        IPAddress clientIp,
        CaptchaChallengeVerifyRequest request,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        if (!settings.Enabled)
        {
            return new CaptchaChallengeVerifyResponse(false, false, "disabled", null, "CAPTCHA integration is disabled.");
        }

        var (subject, state) = await new SecuritySubjectService(db, timeProvider)
            .ResolveOrCreateIpAsync(clientIp, null, null, null, cancellationToken);
        state.ChallengeAttempts++;
        state.UpdatedAtUtc = Now();
        await db.SaveChangesAsync(cancellationToken);

        var verify = await VerifyTokenWithCapAsync(settings, request.Token, cancellationToken);
        if (verify.Success)
        {
            await ClearChallengeAfterSolveAsync(subject, state, settings, cancellationToken);
            var redirect = await SafeReturnUrlAsync(request.ReturnUrl, cancellationToken);
            return new CaptchaChallengeVerifyResponse(true, true, verify.Status, redirect, null);
        }

        await RecordFailedSolveAsync(subject, state, settings, verify.Status, verify.Error, cancellationToken);
        return new CaptchaChallengeVerifyResponse(
            false,
            false,
            verify.Status,
            null,
            verify.Status == "unavailable" ? "CAPTCHA verification is temporarily unavailable." : "CAPTCHA verification failed.");
    }

    public async Task MarkChallengeRequiredAsync(
        IPAddress clientIp,
        Guid? resourceId,
        string reason,
        CancellationToken cancellationToken = default)
    {
        var (subject, state) = await new SecuritySubjectService(db, timeProvider)
            .ResolveOrCreateIpAsync(clientIp, null, null, null, cancellationToken);
        if (!state.ChallengeRequired)
        {
            state.ChallengeRequired = true;
            state.ChallengeRequiredSinceUtc = Now();
            state.ChallengeReason = reason;
            state.ChallengeResourceId = resourceId;
            state.UpdatedAtUtc = Now();
            subject.CurrentState = SecuritySubjectStateNames.Challenged;
            db.SecurityEvents.Add(new SecurityEventEntity
            {
                Category = "captcha",
                Action = "challenge_required",
                ClientIp = subject.NormalizedValue,
                SubjectType = subject.SubjectType,
                SubjectValue = subject.SubjectValue,
                NormalizedSubjectValue = subject.NormalizedValue,
                ResourceId = resourceId,
                EventType = "challenge_required",
                Decision = "challenge",
                Source = "hashi_security_decision",
                Reason = reason,
                OccurredAtUtc = Now(),
            });
            await db.SaveChangesAsync(cancellationToken);
        }
    }

    public async Task CountProtectedHitWhileChallengedAsync(
        SecuritySubjectEntity subject,
        SecuritySubjectStateEntity state,
        Guid? resourceId,
        string reason,
        CancellationToken cancellationToken = default)
    {
        state.RequestsWhileChallenged++;
        state.UpdatedAtUtc = Now();

        var policy = await GetOrCreateSecurityPolicyAsync(cancellationToken);
        if (state.RequestsWhileChallenged >= policy.FirewallBlockThresholdWhileChallenged)
        {
            state.FirewallBlockedUntilUtc ??= Now().Add(EvaluateFirewallBlockDuration(policy));
            state.LastEscalationReason = "captcha_ignored_firewall_threshold";
            state.LastEscalationAtUtc = Now();
            subject.CurrentState = SecuritySubjectStateNames.FirewallBlocked;
        }
        else if (state.RequestsWhileChallenged >= policy.ChallengeIgnoredThreshold)
        {
            state.SoftBlockedUntilUtc ??= Now().Add(EvaluateSoftBlockDuration(policy));
            state.LastEscalationReason = "captcha_ignored_soft_threshold";
            state.LastEscalationAtUtc = Now();
            subject.CurrentState = SecuritySubjectStateNames.SoftBlocked;
        }

        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "captcha",
            Action = "challenge_ignored",
            ClientIp = subject.NormalizedValue,
            SubjectType = subject.SubjectType,
            SubjectValue = subject.SubjectValue,
            NormalizedSubjectValue = subject.NormalizedValue,
            ResourceId = resourceId,
            EventType = "challenge_ignored",
            Decision = "challenge",
            Source = "hashi_forward_auth",
            Reason = reason,
            OccurredAtUtc = Now(),
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<bool> IsPublicChallengeResourceAsync(Guid? resourceId, CancellationToken cancellationToken = default)
    {
        if (resourceId is not Guid id)
        {
            return false;
        }

        var settings = await db.CaptchaSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        return settings?.Enabled == true && settings.PublicChallengeResourceId == id;
    }

    public static string? BuildCapWidgetEndpoint(CaptchaSettingsEntity settings)
    {
        if (string.IsNullOrWhiteSpace(settings.PublicChallengeBaseUrl) || string.IsNullOrWhiteSpace(settings.SiteKey))
        {
            return null;
        }

        var baseText = settings.PublicChallengeBaseUrl.Trim().TrimEnd('/');
        var siteSegment = Uri.EscapeDataString(settings.SiteKey.Trim());
        return baseText.EndsWith("/" + siteSegment, StringComparison.OrdinalIgnoreCase)
            ? baseText + "/"
            : $"{baseText}/{siteSegment}/";
    }

    private async Task<CapVerifyResult> VerifyTokenWithCapAsync(
        CaptchaSettingsEntity settings,
        string token,
        CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(token))
        {
            return CapVerifyResult.Failed("Missing Cap token.");
        }

        if (string.IsNullOrWhiteSpace(settings.PublicChallengeBaseUrl)
            || string.IsNullOrWhiteSpace(settings.SiteKey)
            || settings.SecretKeySecretId is not Guid secretId
            || !Uri.TryCreate(settings.PublicChallengeBaseUrl, UriKind.Absolute, out var baseUri))
        {
            return CapVerifyResult.Unavailable("Cap settings are incomplete.");
        }

        var secretBytes = await secrets.DecryptForServiceSyncAsync(secretId, cancellationToken);
        if (secretBytes is null)
        {
            return CapVerifyResult.Unavailable("Cap secret key is not available to the service-sync vault.");
        }

        var secretKey = Encoding.UTF8.GetString(secretBytes);
        try
        {
            return await capClient.VerifyAsync(
                baseUri,
                settings.SiteKey,
                secretKey,
                token,
                TimeSpan.FromSeconds(settings.VerificationTimeoutSeconds),
                cancellationToken);
        }
        finally
        {
            Array.Clear(secretBytes);
        }
    }

    private async Task ClearChallengeAfterSolveAsync(
        SecuritySubjectEntity subject,
        SecuritySubjectStateEntity state,
        CaptchaSettingsEntity settings,
        CancellationToken cancellationToken)
    {
        state.ChallengeRequired = false;
        state.ChallengeRequiredSinceUtc = null;
        state.ChallengeReason = null;
        state.ChallengeResourceId = null;
        state.SuccessfulChallengeCount++;
        state.LastChallengeSolvedAtUtc = Now();
        state.UpdatedAtUtc = Now();
        if (SecuritySubjectStateNames.Normalize(subject.CurrentState) == SecuritySubjectStateNames.Challenged)
        {
            subject.CurrentState = SecuritySubjectStateNames.Observed;
        }

        await ApplyBucketSolveSemanticsAsync(subject.NormalizedValue, settings, cancellationToken);
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "captcha",
            Action = "challenge_solved",
            ClientIp = subject.NormalizedValue,
            SubjectType = subject.SubjectType,
            SubjectValue = subject.SubjectValue,
            NormalizedSubjectValue = subject.NormalizedValue,
            EventType = "challenge_solved",
            Decision = "allow",
            Source = "hashi_edge_challenge",
            OccurredAtUtc = Now(),
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task ApplyBucketSolveSemanticsAsync(
        string normalizedSubjectValue,
        CaptchaSettingsEntity settings,
        CancellationToken cancellationToken)
    {
        var buckets = await db.SecurityRequestBuckets
            .Where(x => x.NormalizedSubjectValue == normalizedSubjectValue)
            .Where(x => x.ChallengedCount > 0 || x.ChallengeIgnoredCount > 0 || x.FailedChallengeCount > 0)
            .ToListAsync(cancellationToken);
        foreach (var bucket in buckets)
        {
            if (settings.ChallengeResetMode == CaptchaChallengeResetModeNames.Reset)
            {
                bucket.ChallengedCount = 0;
                bucket.ChallengeIgnoredCount = 0;
                bucket.FailedChallengeCount = 0;
                continue;
            }

            if (settings.ChallengeResetMode == CaptchaChallengeResetModeNames.Decay)
            {
                var keepPercent = Math.Clamp(100 - settings.ChallengeDecayPercent, 0, 100) / 100d;
                bucket.ChallengedCount = (long)Math.Ceiling(bucket.ChallengedCount * keepPercent);
                bucket.ChallengeIgnoredCount = (long)Math.Ceiling(bucket.ChallengeIgnoredCount * keepPercent);
                bucket.FailedChallengeCount = (long)Math.Ceiling(bucket.FailedChallengeCount * keepPercent);
            }
        }
    }

    private async Task RecordFailedSolveAsync(
        SecuritySubjectEntity subject,
        SecuritySubjectStateEntity state,
        CaptchaSettingsEntity settings,
        string status,
        string? error,
        CancellationToken cancellationToken)
    {
        state.FailedChallengeCount++;
        if (state.FailedChallengeCount >= settings.MaximumFailuresBeforeEscalation)
        {
            state.SoftBlockedUntilUtc ??= Now().AddMinutes(10);
            state.LastEscalationReason = "captcha_failed_threshold";
            state.LastEscalationAtUtc = Now();
            subject.CurrentState = SecuritySubjectStateNames.SoftBlocked;
        }

        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "captcha",
            Action = status == "unavailable" ? "challenge_unavailable" : "challenge_failed",
            ClientIp = subject.NormalizedValue,
            SubjectType = subject.SubjectType,
            SubjectValue = subject.SubjectValue,
            NormalizedSubjectValue = subject.NormalizedValue,
            EventType = status == "unavailable" ? "challenge_unavailable" : "challenge_failed",
            Decision = "challenge",
            Source = "hashi_edge_challenge",
            Reason = error,
            OccurredAtUtc = Now(),
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task EnsurePublicChallengeResourceAsync(CaptchaSettingsEntity settings, CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(settings.PublicChallengeDomain))
        {
            throw new InvalidOperationException("Public challenge resource domain is required when CAPTCHA is enabled.");
        }

        var system = await db.SystemResources
            .Include(x => x.Resource)
            .SingleOrDefaultAsync(x => x.SystemKey == PublicChallengeSystemKey, cancellationToken);
        var resource = system?.Resource;
        if (resource is null && settings.PublicChallengeResourceId is Guid existingId)
        {
            resource = await db.Resources.SingleOrDefaultAsync(x => x.Id == existingId, cancellationToken);
        }

        if (resource is null)
        {
            resource = new ResourceEntity
            {
                Name = "Hashi CAPTCHA Challenge",
                Slug = "hashi-captcha-challenge",
                Kind = "https",
                DomainMode = "custom",
                TargetScheme = "http",
                TargetHost = "127.0.0.1",
                TargetPort = 8080,
                DashboardEnabled = false,
                StatusEnabled = false,
                ExtraMiddlewaresJson = "[]",
                WafMode = "off",
                ForwardAuthPolicy = "off",
            };
            db.Resources.Add(resource);
            await db.SaveChangesAsync(cancellationToken);
        }

        resource.Enabled = true;
        resource.IsSystem = true;
        resource.Ownership = ResourceOwnershipNames.System;
        resource.OwningWorkflow = PublicChallengeWorkflow;
        resource.DeletionPolicy = ResourceDeletionPolicyNames.RequiredForAccess;
        resource.DomainMode = "custom";
        resource.Domain = settings.PublicChallengeDomain;
        resource.ForwardAuthPolicy = "off";
        resource.WafMode = "off";
        resource.PathPrefix = null;
        resource.PathRewrite = null;
        resource.PathRewriteMode = null;
        resource.UpdatedAtUtc = Now();
        settings.PublicChallengeResourceId = resource.Id;

        if (system is null)
        {
            db.SystemResources.Add(new SystemResourceEntity
            {
                ResourceId = resource.Id,
                SystemKey = PublicChallengeSystemKey,
                OwningWorkflow = PublicChallengeWorkflow,
                RequiredForAppAccess = true,
            });
        }

        await UpsertChallengeRoutesAsync(resource.Id, cancellationToken);
    }

    private async Task EnsureOptionalAdminResourceAsync(CaptchaSettingsEntity settings, CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(settings.CapAdminDomain))
        {
            return;
        }

        if (settings.CapAdminResourceId is Guid existingId
            && await db.Resources.AnyAsync(x => x.Id == existingId, cancellationToken))
        {
            return;
        }

        var resource = new ResourceEntity
        {
            Name = "Cap Admin",
            Slug = "cap-admin",
            Kind = "https",
            DomainMode = "custom",
            Domain = settings.CapAdminDomain,
            TargetScheme = "http",
            TargetHost = "127.0.0.1",
            TargetPort = 3000,
            ForwardAuthPolicy = "sso_required",
            WafMode = "detect_only",
            DashboardEnabled = false,
            StatusEnabled = false,
            Ownership = ResourceOwnershipNames.UserCreated,
            DeletionPolicy = ResourceDeletionPolicyNames.Optional,
        };
        db.Resources.Add(resource);
        settings.CapAdminResourceId = resource.Id;
    }

    private async Task UpsertChallengeRoutesAsync(Guid resourceId, CancellationToken cancellationToken)
    {
        var existing = await db.ResourceRoutes.Where(x => x.ResourceId == resourceId).ToListAsync(cancellationToken);
        db.ResourceRoutes.RemoveRange(existing);
        db.ResourceRoutes.AddRange(
            ChallengeRoute(resourceId, 300, "/api/edge-challenge"),
            ChallengeRoute(resourceId, 200, "/challenge"),
            ChallengeRoute(resourceId, 100, "/_app"),
            ChallengeRoute(resourceId, 90, "/favicon.svg"));
    }

    private static ResourceRouteEntity ChallengeRoute(Guid resourceId, int priority, string path) => new()
    {
        ResourceId = resourceId,
        Enabled = true,
        Priority = priority,
        PathMatchType = "prefix",
        PathValue = path,
        TargetScheme = "http",
        TargetHost = "127.0.0.1",
        TargetPort = 8080,
        ExtraMiddlewaresJson = "[]",
    };

    private async Task ReleasePublicChallengeResourceAsync(CaptchaSettingsEntity settings, CancellationToken cancellationToken)
    {
        var resourceId = settings.PublicChallengeResourceId;
        if (resourceId is not Guid id)
        {
            return;
        }

        var resource = await db.Resources.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (resource is null)
        {
            return;
        }

        resource.Enabled = false;
        resource.IsSystem = false;
        resource.Ownership = ResourceOwnershipNames.Managed;
        resource.DeletionPolicy = ResourceDeletionPolicyNames.Optional;
        resource.UpdatedAtUtc = Now();
    }

    private async Task<string> SafeReturnUrlAsync(string? returnUrl, CancellationToken cancellationToken)
    {
        if (!Uri.TryCreate(returnUrl, UriKind.Absolute, out var uri)
            || uri.Scheme is not ("https" or "http"))
        {
            return "/";
        }

        var normalizedHost = NormalizeHost(uri.Host);
        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        if (!string.IsNullOrWhiteSpace(settings?.AdminDomain)
            && string.Equals(normalizedHost, NormalizeHost(settings.AdminDomain), StringComparison.OrdinalIgnoreCase))
        {
            return uri.ToString();
        }

        var resources = await db.Resources.AsNoTracking().Where(x => x.Enabled).ToListAsync(cancellationToken);
        var resource = resources.FirstOrDefault(x => string.Equals(
            ResourceDomainResolver.Resolve(x.DomainMode, x.Domain, x.Slug, settings?.RootDomain),
            normalizedHost,
            StringComparison.OrdinalIgnoreCase));
        if (resource is null)
        {
            return "/";
        }

        return string.Equals(resource.OwningWorkflow, PublicChallengeWorkflow, StringComparison.OrdinalIgnoreCase)
            ? "/"
            : uri.ToString();
    }

    private static void ValidateSettingsRequest(CaptchaSettingsRequest request, CaptchaSettingsEntity existing)
    {
        if (!request.Enabled)
        {
            return;
        }

        if (string.IsNullOrWhiteSpace(request.PublicChallengeBaseUrl))
        {
            throw new InvalidOperationException("Cap public challenge base URL is required when CAPTCHA is enabled.");
        }

        if (!Uri.TryCreate(request.PublicChallengeBaseUrl, UriKind.Absolute, out var baseUri)
            || baseUri.Scheme is not ("https" or "http"))
        {
            throw new InvalidOperationException("Cap public challenge base URL must be an absolute HTTP or HTTPS URL.");
        }

        if (string.IsNullOrWhiteSpace(request.SiteKey))
        {
            throw new InvalidOperationException("Cap site key is required when CAPTCHA is enabled.");
        }

        if (string.IsNullOrWhiteSpace(request.SecretKey)
            && request.SecretKeySecretId is null
            && existing.SecretKeySecretId is null)
        {
            throw new InvalidOperationException("Cap secret key is required when CAPTCHA is enabled.");
        }

        var domain = NormalizeDomain(request.PublicChallengeDomain);
        if (string.IsNullOrWhiteSpace(domain))
        {
            throw new InvalidOperationException("Public challenge resource domain is required when CAPTCHA is enabled.");
        }

        if (domain.EndsWith(".home.arpa", StringComparison.OrdinalIgnoreCase)
            || string.Equals(domain, "hashi.home.arpa", StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException("The public challenge resource must use a real reverse-proxy domain.");
        }
    }

    private async Task<CaptchaSettingsEntity> GetOrCreateSettingsAsync(CancellationToken cancellationToken)
    {
        var settings = await db.CaptchaSettings.SingleOrDefaultAsync(cancellationToken);
        if (settings is not null)
        {
            return settings;
        }

        settings = new CaptchaSettingsEntity();
        db.CaptchaSettings.Add(settings);
        await db.SaveChangesAsync(cancellationToken);
        return settings;
    }

    private async Task<SecurityPolicySettingsEntity> GetOrCreateSecurityPolicyAsync(CancellationToken cancellationToken)
    {
        var settings = await db.SecurityPolicySettings.SingleOrDefaultAsync(cancellationToken);
        if (settings is not null)
        {
            return settings;
        }

        settings = new SecurityPolicySettingsEntity();
        db.SecurityPolicySettings.Add(settings);
        await db.SaveChangesAsync(cancellationToken);
        return settings;
    }

    private TimeSpan EvaluateSoftBlockDuration(SecurityPolicySettingsEntity settings)
        => banPolicies.Evaluate(ParsePolicy(settings.DefaultSoftBlockPolicyJson), 1).Duration ?? TimeSpan.FromMinutes(10);

    private TimeSpan EvaluateFirewallBlockDuration(SecurityPolicySettingsEntity settings)
        => banPolicies.Evaluate(ParsePolicy(settings.DefaultFirewallBlockPolicyJson), 1).Duration ?? TimeSpan.FromHours(1);

    private static BanDurationPolicy ParsePolicy(string json)
        => JsonSerializer.Deserialize<BanDurationPolicy>(json, new JsonSerializerOptions(JsonSerializerDefaults.Web))
            ?? JsonSerializer.Deserialize<BanDurationPolicy>(BanDurationPolicyDefaults.SoftBlockJson, new JsonSerializerOptions(JsonSerializerDefaults.Web))!;

    private static CaptchaSettingsResponse ToResponse(CaptchaSettingsEntity settings)
        => new(
            settings.Enabled,
            settings.PublicChallengeBaseUrl,
            settings.SiteKey,
            settings.SecretKeySecretId.HasValue,
            settings.VerificationTimeoutSeconds,
            settings.InstrumentationExpected,
            settings.HeadlessDetectionExpected,
            settings.CapAdminResourceId,
            settings.CapAdminDomain,
            settings.PublicChallengeResourceId,
            settings.PublicChallengeDomain,
            settings.ChallengeResetMode,
            settings.ChallengeDecayPercent,
            settings.MinimumRepeatChallengeSeconds,
            settings.MaximumFailuresBeforeEscalation,
            settings.MaximumRequestsWhileChallenged,
            settings.UpdatedAtUtc);

    private DateTimeOffset Now() => timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;

    private static string? NormalizeNullable(string? value)
        => string.IsNullOrWhiteSpace(value) ? null : value.Trim();

    private static string? NormalizeNullableUrl(string? value)
        => string.IsNullOrWhiteSpace(value) ? null : value.Trim().TrimEnd('/');

    private static string? NormalizeDomain(string? value)
        => string.IsNullOrWhiteSpace(value) ? null : value.Trim().TrimEnd('.').ToLowerInvariant();

    private static string NormalizeHost(string value)
        => value.Trim().TrimEnd('.').ToLowerInvariant();

    private static string NormalizeChallengeResetMode(string? value)
        => value?.Trim().ToLowerInvariant() switch
        {
            CaptchaChallengeResetModeNames.Reset => CaptchaChallengeResetModeNames.Reset,
            CaptchaChallengeResetModeNames.None => CaptchaChallengeResetModeNames.None,
            _ => CaptchaChallengeResetModeNames.Decay,
        };
}
