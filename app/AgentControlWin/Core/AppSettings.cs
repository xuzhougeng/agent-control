using System.IO;
using System.Text.Json;

namespace AgentControlWin.Core;

/// <summary>
/// Simple file-based settings store for unpackaged apps.
/// Stores settings as JSON in %APPDATA%\AgentControlWin\settings.json.
/// </summary>
public static class AppSettings
{
    private static readonly string SettingsPath = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "AgentControlWin", "settings.json");

    private static Dictionary<string, object?> _cache = Load();

    private static Dictionary<string, object?> Load()
    {
        try
        {
            if (File.Exists(SettingsPath))
            {
                var json = File.ReadAllText(SettingsPath);
                return JsonSerializer.Deserialize<Dictionary<string, object?>>(json) ?? [];
            }
        }
        catch { }
        return [];
    }

    private static void Save()
    {
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(SettingsPath)!);
            File.WriteAllText(SettingsPath, JsonSerializer.Serialize(_cache));
        }
        catch { }
    }

    public static string? GetString(string key, string? defaultValue = null)
    {
        if (_cache.TryGetValue(key, out var val))
            return val?.ToString() ?? defaultValue;
        return defaultValue;
    }

    public static bool GetBool(string key, bool defaultValue = false)
    {
        if (_cache.TryGetValue(key, out var val))
        {
            if (val is bool b) return b;
            if (val is JsonElement je && je.ValueKind == JsonValueKind.True) return true;
            if (val is JsonElement je2 && je2.ValueKind == JsonValueKind.False) return false;
            if (val?.ToString() is string s) return s.Equals("true", StringComparison.OrdinalIgnoreCase);
        }
        return defaultValue;
    }

    public static void Set(string key, object? value)
    {
        _cache[key] = value;
        Save();
    }
}
