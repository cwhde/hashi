using System.Security.Cryptography;
using System.Text;

namespace Hashi.Infrastructure.Crypto;

public static class KeyDerivation
{
    private const int Pbkdf2Iterations = 600_000;
    private const int SaltSizeBytes = 32;

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
        var salt = RandomNumberGenerator.GetBytes(SaltSizeBytes);
        var hash = Pbkdf2Hash(recoveryKey, salt);
        return $"{Convert.ToHexString(salt).ToLowerInvariant()}:{Convert.ToHexString(hash).ToLowerInvariant()}";
    }

    public static bool VerifyRecoveryKeyHash(string recoveryKey, string storedHash)
    {
        var parts = storedHash.Split(':');
        if (parts.Length != 2)
        {
            return false;
        }

        var salt = Convert.FromHexString(parts[0]);
        var expectedHash = Convert.FromHexString(parts[1]);
        var computedHash = Pbkdf2Hash(recoveryKey, salt);
        return CryptographicOperations.FixedTimeEquals(computedHash, expectedHash);
    }

    private static byte[] Pbkdf2Hash(string password, byte[] salt)
    {
        return Rfc2898DeriveBytes.Pbkdf2(
            password,
            salt,
            Pbkdf2Iterations,
            HashAlgorithmName.SHA256,
            32);
    }

    private static byte[] DeriveKey(string purpose, ReadOnlySpan<byte> input)
    {
        var salt = SHA256.HashData(Encoding.UTF8.GetBytes(purpose));
        var inputBytes = input.ToArray();
        return HKDF.DeriveKey(HashAlgorithmName.SHA256, inputBytes, 32, salt, Encoding.UTF8.GetBytes(purpose));
    }
}
