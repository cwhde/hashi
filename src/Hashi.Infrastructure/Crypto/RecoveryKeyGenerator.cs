using System.Security.Cryptography;

namespace Hashi.Infrastructure.Crypto;

public static class RecoveryKeyGenerator
{
    private const string Alphabet = "abcdefghijklmnopqrstuvwxyz234567";

    public static string Generate(int groups = 8, int groupLength = 4)
    {
        var chars = new char[(groups * groupLength) + (groups - 1)];
        var index = 0;
        for (var group = 0; group < groups; group++)
        {
            if (group > 0)
            {
                chars[index++] = '-';
            }

            for (var i = 0; i < groupLength; i++)
            {
                chars[index++] = Alphabet[RandomNumberGenerator.GetInt32(Alphabet.Length)];
            }
        }

        return new string(chars);
    }
}
