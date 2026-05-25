using System.Security.Cryptography;

namespace Hashi.Infrastructure.Crypto;

public sealed record AesGcmBlob(byte[] Nonce, byte[] Tag, byte[] Ciphertext)
{
    public byte[] ToBlob()
    {
        var blob = new byte[Nonce.Length + Tag.Length + Ciphertext.Length];
        Buffer.BlockCopy(Nonce, 0, blob, 0, Nonce.Length);
        Buffer.BlockCopy(Tag, 0, blob, Nonce.Length, Tag.Length);
        Buffer.BlockCopy(Ciphertext, 0, blob, Nonce.Length + Tag.Length, Ciphertext.Length);
        return blob;
    }

    public static AesGcmBlob FromBlob(ReadOnlySpan<byte> blob)
    {
        const int nonceSize = 12;
        const int tagSize = 16;
        if (blob.Length <= nonceSize + tagSize)
        {
            throw new CryptographicException("Ciphertext blob is too short.");
        }

        return new AesGcmBlob(
            blob[..nonceSize].ToArray(),
            blob.Slice(nonceSize, tagSize).ToArray(),
            blob[(nonceSize + tagSize)..].ToArray());
    }
}

public static class AesGcmCipher
{
    private const int NonceSize = 12;
    private const int TagSize = 16;

    public static AesGcmBlob Encrypt(ReadOnlySpan<byte> plaintext, ReadOnlySpan<byte> key)
    {
        if (key.Length != 32)
        {
            throw new ArgumentException("AES-256-GCM requires a 32-byte key.", nameof(key));
        }

        var nonce = RandomNumberGenerator.GetBytes(NonceSize);
        var ciphertext = new byte[plaintext.Length];
        var tag = new byte[TagSize];
        using var aes = new AesGcm(key, TagSize);
        aes.Encrypt(nonce, plaintext, ciphertext, tag);
        return new AesGcmBlob(nonce, tag, ciphertext);
    }

    public static byte[] Decrypt(ReadOnlySpan<byte> blob, ReadOnlySpan<byte> key)
    {
        if (key.Length != 32)
        {
            throw new ArgumentException("AES-256-GCM requires a 32-byte key.", nameof(key));
        }

        var parsed = AesGcmBlob.FromBlob(blob);
        var plaintext = new byte[parsed.Ciphertext.Length];
        using var aes = new AesGcm(key, TagSize);
        aes.Decrypt(parsed.Nonce, parsed.Ciphertext, parsed.Tag, plaintext);
        return plaintext;
    }
}
