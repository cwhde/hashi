using System.Security.Cryptography;
using System.Text;
using Fido2NetLib;
using Fido2NetLib.Objects;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Auth;

public sealed class PasskeyAuthService(
    IFido2 fido2,
    HashiDbContext db,
    SetupStateService setupState,
    AuditService audit,
    ILogger<PasskeyAuthService> logger)
{
    public async Task<CredentialCreateOptions> BeginRegistrationAsync(
        string nickname,
        bool allowAfterSetupComplete = false,
        CancellationToken cancellationToken = default)
    {
        var state = await setupState.GetOrCreateAsync(cancellationToken);
        if (state.IsComplete && !allowAfterSetupComplete)
        {
            throw new InvalidOperationException("Passkey registration is only available during setup.");
        }

        var existing = await db.PasskeyCredentials
            .Select(x => new PublicKeyCredentialDescriptor(x.CredentialId))
            .ToListAsync(cancellationToken);

        var user = new Fido2User
        {
            DisplayName = "Hashi Admin",
            Name = "admin",
            Id = Encoding.UTF8.GetBytes("hashi-admin"),
        };

        var authenticatorSelection = new AuthenticatorSelection
        {
            RequireResidentKey = false,
            UserVerification = UserVerificationRequirement.Required,
        };

        return fido2.RequestNewCredential(
            user,
            existing,
            authenticatorSelection,
            AttestationConveyancePreference.None);
    }

    public async Task<PasskeyRegistrationResult> CompleteRegistrationAsync(
        AuthenticatorAttestationRawResponse attestation,
        CredentialCreateOptions originalOptions,
        string nickname,
        bool clientReportsPrfSupported,
        CancellationToken cancellationToken = default)
    {
        var success = await fido2.MakeNewCredentialAsync(
            attestation,
            originalOptions,
            (_, _) => Task.FromResult(true));

        var entity = new PasskeyCredentialEntity
        {
            CredentialId = success.Result!.CredentialId,
            PublicKey = success.Result.PublicKey,
            SignCount = success.Result.Counter,
            Nickname = string.IsNullOrWhiteSpace(nickname) ? "Primary passkey" : nickname.Trim(),
            PrfSupported = clientReportsPrfSupported,
            PrfSalt = clientReportsPrfSupported ? RandomNumberGenerator.GetBytes(32) : null,
        };

        db.PasskeyCredentials.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "auth",
            "passkey_registered",
            subjectType: "passkey",
            subjectId: entity.Id.ToString(),
            metadata: new { prfSupported = clientReportsPrfSupported },
            cancellationToken: cancellationToken);
        logger.LogInformation("Registered passkey {PasskeyId} (PRF supported: {PrfSupported})", entity.Id, clientReportsPrfSupported);

        return new PasskeyRegistrationResult(entity.Id, clientReportsPrfSupported);
    }

    public async Task<AssertionOptions> BeginLoginAsync(CancellationToken cancellationToken = default)
    {
        var credentials = await db.PasskeyCredentials.ToListAsync(cancellationToken);
        if (credentials.Count == 0)
        {
            throw new InvalidOperationException("No passkeys are registered.");
        }

        var allowed = credentials.Select(x => new PublicKeyCredentialDescriptor(x.CredentialId)).ToList();
        return fido2.GetAssertionOptions(allowed, UserVerificationRequirement.Required);
    }

    public async Task<PasskeyLoginResult> CompleteLoginAsync(
        AuthenticatorAssertionRawResponse assertion,
        AssertionOptions originalOptions,
        byte[]? clientPrfOutput,
        CancellationToken cancellationToken = default)
    {
        var credentialId = assertion.RawId;
        var credentialIdBase64 = Convert.ToBase64String(credentialId);
        var stored = await db.PasskeyCredentials.SingleOrDefaultAsync(
            x => x.CredentialIdBase64 == credentialIdBase64,
            cancellationToken)
            ?? throw new InvalidOperationException("Unknown passkey credential.");

        var success = await fido2.MakeAssertionAsync(
            assertion,
            originalOptions,
            stored.PublicKey,
            stored.SignCount,
            (_, _) => Task.FromResult(true));

        stored.SignCount = success.Counter;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("auth", "passkey_login", subjectType: "passkey", subjectId: stored.Id.ToString(), cancellationToken: cancellationToken);

        return new PasskeyLoginResult(stored.Id, clientPrfOutput);
    }

    public async Task<IReadOnlyList<PasskeyCredentialEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.PasskeyCredentials.AsNoTracking().OrderBy(x => x.CreatedAtUtc).ToListAsync(cancellationToken);

    public async Task<bool> DeleteAsync(Guid credentialId, CancellationToken cancellationToken = default)
    {
        var count = await db.PasskeyCredentials.CountAsync(cancellationToken);
        if (count <= 1)
        {
            throw new InvalidOperationException("At least one passkey must remain.");
        }

        var credential = await db.PasskeyCredentials.SingleOrDefaultAsync(x => x.Id == credentialId, cancellationToken);
        if (credential is null)
        {
            return false;
        }

        db.PasskeyCredentials.Remove(credential);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("auth", "passkey_removed", subjectType: "passkey", subjectId: credentialId.ToString(), cancellationToken: cancellationToken);
        return true;
    }
}

public sealed record PasskeyRegistrationResult(Guid CredentialId, bool PrfSupported);

public sealed record PasskeyLoginResult(Guid CredentialId, byte[]? PrfOutput);
