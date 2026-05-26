using Fido2NetLib;
using Microsoft.Extensions.Caching.Memory;

namespace Hashi.Infrastructure.Auth;

public sealed class WebAuthnChallengeStore(IMemoryCache cache)
{
    private static readonly TimeSpan Ttl = TimeSpan.FromMinutes(5);

    public void StoreRegistration(string sessionKey, CredentialCreateOptions options)
        => cache.Set(RegistrationKey(sessionKey), options, Ttl);

    public CredentialCreateOptions? GetRegistration(string sessionKey)
        => cache.Get<CredentialCreateOptions>(RegistrationKey(sessionKey));

    public void StoreLogin(string sessionKey, AssertionOptions options)
        => cache.Set(LoginKey(sessionKey), options, Ttl);

    public AssertionOptions? GetLogin(string sessionKey)
        => cache.Get<AssertionOptions>(LoginKey(sessionKey));

    private static string RegistrationKey(string sessionKey) => $"webauthn:register:{sessionKey}";

    private static string LoginKey(string sessionKey) => $"webauthn:login:{sessionKey}";
}
