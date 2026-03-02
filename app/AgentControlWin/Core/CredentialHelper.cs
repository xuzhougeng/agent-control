using System.IO;
using System.Security.Cryptography;
using System.Text;

namespace AgentControlWin.Core;

/// <summary>
/// Stores the auth token securely using DPAPI (ProtectedData).
/// Data is encrypted per-user and written to %APPDATA%\AgentControlWin\token.dat.
/// Mirrors KeychainHelper.swift for macOS.
/// </summary>
public static class CredentialHelper
{
    private static readonly string DataDir = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "AgentControlWin");

    private static string TokenPath => Path.Combine(DataDir, "token.dat");

    public static void SaveToken(string token)
    {
        Directory.CreateDirectory(DataDir);
        var plainBytes = Encoding.UTF8.GetBytes(token);
        var encrypted = ProtectedData.Protect(plainBytes, null, DataProtectionScope.CurrentUser);
        File.WriteAllBytes(TokenPath, encrypted);
    }

    public static string? LoadToken()
    {
        if (!File.Exists(TokenPath)) return null;
        try
        {
            var encrypted = File.ReadAllBytes(TokenPath);
            var plain = ProtectedData.Unprotect(encrypted, null, DataProtectionScope.CurrentUser);
            return Encoding.UTF8.GetString(plain);
        }
        catch
        {
            return null;
        }
    }

    public static void DeleteToken()
    {
        if (File.Exists(TokenPath))
            File.Delete(TokenPath);
    }
}
