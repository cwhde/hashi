using System.Security.Cryptography;
using System.Text;

namespace Hashi.Infrastructure.Crypto;

public static class KeyDerivation
{
    public static byte[] DeriveRecoveryWrapKey(string recoveryKey)
        => DeriveKey("hashi:vault:recovery", Encoding.UTF8.GetBytes(recoveryKey));

    public static byte[] DerivePrfWrapKey(ReadOnlySpan<byte> prfOutput, ReadOnlySpan<byte> credentialId)
    {
        var input = new byte[prfOutput.Length + credentialId.Length];
        prfOutput.CopyTo(input);
        credentialId.CopyTo(input.AsSpan(prfOutput.Length));
        return DeriveKey("hashi:vault:prf", input);
    }

    public static byte[] DeriveServiceSyncWrapKey(ReadOnlySpan<byte> deploymentSecret)
        => DeriveKey("hashi:vault:service-sync", deploymentSecret);

    public static string HashRecoveryKeyForVerification(string recoveryKey)
    {
        var hash = SHA256.HashData(Encoding.UTF8.GetBytes(recoveryKey));
        return Convert.ToHexString(hash).ToLowerInvariant();
    }

    private static byte[] DeriveKey(string purpose, ReadOnlySpan<byte> input)
    {
        var salt = SHA256.HashData(Encoding.UTF8.GetBytes(purpose));
        var inputBytes = input.ToArray();
        return HKDF.DeriveKey(HashAlgorithmName.SHA256, inputBytes, 32, salt, Encoding.UTF8.GetBytes(purpose));
    }
}
