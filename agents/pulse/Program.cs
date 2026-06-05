using System;
using System.Collections.Generic;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace Hashi.Pulse;

public static class Program
{
    internal const string Version = "0.1.0";

    public static async Task<int> Main(string[] args)
    {
        var (config, configErr) = LoadConfig(args);
        if (configErr != null)
        {
            Console.Error.WriteLine($"configuration error: {configErr}");
            return 1;
        }

        var cts = new CancellationTokenSource();

        Console.CancelKeyPress += OnCancelKeyPress;
        AppDomain.CurrentDomain.ProcessExit += OnProcessExit;

        void OnCancelKeyPress(object? sender, ConsoleCancelEventArgs e)
        {
            e.Cancel = true;
            try { cts.Cancel(); } catch (ObjectDisposedException) { }
        }

        void OnProcessExit(object? sender, EventArgs e)
        {
            try { cts.Cancel(); } catch (ObjectDisposedException) { }
        }

        try
        {
            using var handler = new HttpClientHandler();
            using var client = new HttpClient(handler);
            client.Timeout = config.Timeout;

            if (config.Once)
            {
                try
                {
                    await SendHeartbeatAsync(client, config, cts.Token);
                    return 0;
                }
                catch (Exception ex)
                {
                    Console.Error.WriteLine($"heartbeat failed: {ex.Message}");
                    return 1;
                }
            }

            while (!cts.IsCancellationRequested)
            {
                try
                {
                    await SendHeartbeatAsync(client, config, cts.Token);
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"heartbeat failed: {ex.Message}");
                }

                try
                {
                    await Task.Delay(config.Interval, cts.Token);
                }
                catch (TaskCanceledException)
                {
                    break;
                }
            }

            return 0;
        }
        finally
        {
            Console.CancelKeyPress -= OnCancelKeyPress;
            AppDomain.CurrentDomain.ProcessExit -= OnProcessExit;
            cts.Dispose();
        }
    }

    internal sealed class Config
    {
        public string ApiUrl { get; set; } = string.Empty;
        public string AgentId { get; set; } = string.Empty;
        public string Token { get; set; } = string.Empty;
        public string SelectedInterface { get; set; } = string.Empty;
        public TimeSpan Interval { get; set; } = TimeSpan.FromMinutes(1);
        public TimeSpan Timeout { get; set; } = TimeSpan.FromSeconds(10);
        public bool Once { get; set; }
    }

    internal static (Config Config, string? Error) LoadConfig(string[] args)
    {
        var config = new Config
        {
            ApiUrl = Environment.GetEnvironmentVariable("HASHI_PULSE_API") ?? string.Empty,
            AgentId = Environment.GetEnvironmentVariable("HASHI_PULSE_AGENT_ID") ?? string.Empty,
            Token = Environment.GetEnvironmentVariable("HASHI_PULSE_TOKEN") ?? string.Empty,
            SelectedInterface = Environment.GetEnvironmentVariable("HASHI_PULSE_INTERFACE") ?? string.Empty,
            Interval = ParseDuration(Environment.GetEnvironmentVariable("HASHI_PULSE_INTERVAL"), TimeSpan.FromMinutes(1)),
            Timeout = ParseDuration(Environment.GetEnvironmentVariable("HASHI_PULSE_TIMEOUT"), TimeSpan.FromSeconds(10)),
            Once = Environment.GetEnvironmentVariable("HASHI_PULSE_ONCE") == "1"
        };

        for (int i = 0; i < args.Length; i++)
        {
            var arg = args[i];
            if (arg.StartsWith("--")) arg = arg[2..];
            else if (arg.StartsWith('-')) arg = arg[1..];
            else continue;

            string value;
            var eqIdx = arg.IndexOf('=');
            if (eqIdx != -1)
            {
                value = arg[(eqIdx + 1)..];
                arg = arg[..eqIdx];
            }
            else
            {
                if (arg.ToLower() == "once")
                {
                    config.Once = true;
                    continue;
                }

                if (i + 1 < args.Length)
                {
                    value = args[i + 1];
                    i++;
                }
                else
                {
                    value = string.Empty;
                }
            }

            switch (arg.ToLower())
            {
                case "api":
                    config.ApiUrl = value;
                    break;
                case "agent-id":
                    config.AgentId = value;
                    break;
                case "token":
                    config.Token = value;
                    break;
                case "interface":
                    config.SelectedInterface = value;
                    break;
                case "interval":
                    config.Interval = ParseDuration(value, config.Interval);
                    break;
                case "timeout":
                    config.Timeout = ParseDuration(value, config.Timeout);
                    break;
                case "once":
                    config.Once = value == "1" || value.Equals("true", StringComparison.OrdinalIgnoreCase) || string.IsNullOrEmpty(value);
                    break;
            }
        }

        config.ApiUrl = config.ApiUrl.Trim().TrimEnd('/');
        config.AgentId = config.AgentId.Trim();
        config.Token = config.Token.Trim();
        config.SelectedInterface = config.SelectedInterface.Trim();

        if (string.IsNullOrEmpty(config.ApiUrl) || string.IsNullOrEmpty(config.AgentId) || string.IsNullOrEmpty(config.Token))
        {
            return (config, "HASHI_PULSE_API, HASHI_PULSE_AGENT_ID, and HASHI_PULSE_TOKEN are required");
        }

        if (config.Interval < TimeSpan.FromSeconds(10))
        {
            config.Interval = TimeSpan.FromSeconds(10);
        }
        if (config.Timeout < TimeSpan.FromSeconds(1))
        {
            config.Timeout = TimeSpan.FromSeconds(1);
        }

        return (config, null);
    }

    internal static TimeSpan ParseDuration(string? value, TimeSpan fallback)
    {
        if (string.IsNullOrWhiteSpace(value)) return fallback;
        value = value.Trim().ToLower();

        try
        {
            if (value.EndsWith("ms"))
            {
                if (double.TryParse(value[..^2], out var ms))
                    return TimeSpan.FromMilliseconds(ms);
            }
            else if (value.EndsWith("s"))
            {
                if (double.TryParse(value[..^1], out var s))
                    return TimeSpan.FromSeconds(s);
            }
            else if (value.EndsWith("m"))
            {
                if (double.TryParse(value[..^1], out var m))
                    return TimeSpan.FromMinutes(m);
            }
            else if (value.EndsWith("h"))
            {
                if (double.TryParse(value[..^1], out var h))
                    return TimeSpan.FromHours(h);
            }
            else
            {
                if (double.TryParse(value, out var sec))
                    return TimeSpan.FromSeconds(sec);
            }
        }
        catch
        {
            // Ignore and fallback
        }
        return fallback;
    }

    internal static bool IsPrivateIPv4(IPAddress ip)
    {
        var bytes = ip.GetAddressBytes();
        if (bytes.Length != 4) return false;

        if (bytes[0] == 10) return true;
        if (bytes[0] == 172 && bytes[1] >= 16 && bytes[1] <= 31) return true;
        if (bytes[0] == 192 && bytes[1] == 168) return true;

        return false;
    }

    internal static bool IsPrivateIPv6(IPAddress ip)
    {
        var bytes = ip.GetAddressBytes();
        return bytes.Length == 16 && (bytes[0] & 0xFE) == 0xFC;
    }

    internal static (List<string> IPv4, List<string> IPv6, string? SelectedInterface, string? SelectedIp) GetPrivateIpCandidates(string preferredInterface)
    {
        var ipv4 = new List<string>();
        var ipv6 = new List<string>();
        string? selectedInterface = null;
        string? selectedIp = null;

        try
        {
            var interfaces = NetworkInterface.GetAllNetworkInterfaces();
            foreach (var iface in interfaces)
            {
                if (iface.OperationalStatus != OperationalStatus.Up ||
                    iface.NetworkInterfaceType == NetworkInterfaceType.Loopback)
                {
                    continue;
                }

                var ipProps = iface.GetIPProperties();
                foreach (var addrInfo in ipProps.UnicastAddresses)
                {
                    var ip = addrInfo.Address;
                    if (ip.AddressFamily == AddressFamily.InterNetwork)
                    {
                        if (IsPrivateIPv4(ip))
                        {
                            var ipStr = ip.ToString();
                            if (!ipv4.Contains(ipStr))
                            {
                                ipv4.Add(ipStr);
                            }

                            if (selectedIp == null && InterfaceMatches(iface.Name, preferredInterface))
                            {
                                selectedInterface = iface.Name;
                                selectedIp = ipStr;
                            }
                        }
                    }
                    else if (ip.AddressFamily == AddressFamily.InterNetworkV6)
                    {
                        if (IsPrivateIPv6(ip))
                        {
                            var ipStr = ip.ToString();
                            var percentIdx = ipStr.IndexOf('%');
                            if (percentIdx != -1)
                            {
                                ipStr = ipStr[..percentIdx];
                            }

                            if (!ipv6.Contains(ipStr))
                            {
                                ipv6.Add(ipStr);
                            }

                            if (selectedIp == null && InterfaceMatches(iface.Name, preferredInterface))
                            {
                                selectedInterface = iface.Name;
                                selectedIp = ipStr;
                            }
                        }
                    }
                }
            }
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Error retrieving network interfaces: {ex.Message}");
        }

        return (ipv4, ipv6, selectedInterface, selectedIp);
    }

    private static bool InterfaceMatches(string name, string preferred)
    {
        return string.IsNullOrEmpty(preferred) || name.Equals(preferred, StringComparison.OrdinalIgnoreCase);
    }

    internal static PulseDockerMetadataRequest? DetectDockerMetadata()
    {
        var hasDockerEnv = File.Exists("/.dockerenv");
        var cgroupContent = ReadFile("/proc/1/cgroup");
        var hasDockerCgroup = cgroupContent.Contains("docker");

        if (!hasDockerEnv && !hasDockerCgroup)
        {
            return null;
        }

        var metadata = new PulseDockerMetadataRequest
        {
            ContainerId = GetDockerContainerId(cgroupContent),
            Image = Environment.GetEnvironmentVariable("HASHI_PULSE_DOCKER_IMAGE"),
            NetworkMode = Environment.GetEnvironmentVariable("HASHI_PULSE_DOCKER_NETWORK_MODE")
        };

        if (string.IsNullOrEmpty(metadata.ContainerId) &&
            string.IsNullOrEmpty(metadata.Image) &&
            string.IsNullOrEmpty(metadata.NetworkMode))
        {
            return null;
        }

        return metadata;
    }

    private static string? GetDockerContainerId(string cgroup)
    {
        var parts = cgroup.Split(new[] { '/', ':', '\n', '\r' }, StringSplitOptions.RemoveEmptyEntries);
        foreach (var rawPart in parts)
        {
            var part = rawPart.Trim();
            if (part.Length >= 12 && IsHex(part))
            {
                return part;
            }
        }

        try
        {
            var hostname = Dns.GetHostName();
            if (!string.IsNullOrEmpty(hostname) && hostname.Length >= 12 && IsHex(hostname))
            {
                return hostname;
            }
        }
        catch
        {
            // Ignore
        }

        return null;
    }

    private static bool IsHex(string value)
    {
        foreach (var ch in value)
        {
            if (!((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')))
            {
                return false;
            }
        }
        return true;
    }

    private static string ReadFile(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                return File.ReadAllText(path);
            }
        }
        catch
        {
            // Ignore
        }
        return string.Empty;
    }

    private static async Task SendHeartbeatAsync(HttpClient client, Config config, CancellationToken cancellationToken)
    {
        string hostname;
        try
        {
            hostname = Dns.GetHostName();
            if (string.IsNullOrWhiteSpace(hostname)) hostname = "unknown";
        }
        catch
        {
            hostname = "unknown";
        }

        var (ipv4, ipv6, selectedInterface, selectedIp) = GetPrivateIpCandidates(config.SelectedInterface);

        var payload = new PulseHeartbeatAuthRequest
        {
            Token = config.Token,
            Version = Version,
            Hostname = hostname,
            PrivateIpv4Candidates = ipv4,
            PrivateIpv6Candidates = ipv6,
            SelectedInterface = selectedInterface,
            SelectedIp = selectedIp,
            Timestamp = DateTimeOffset.UtcNow,
            Docker = DetectDockerMetadata()
        };

        var json = JsonSerializer.Serialize(payload, PulseJsonContext.Default.PulseHeartbeatAuthRequest);
        var content = new StringContent(json, System.Text.Encoding.UTF8, "application/json");

        var url = $"{config.ApiUrl}/api/pulse/{config.AgentId}/heartbeat";

        using var request = new HttpRequestMessage(HttpMethod.Post, url);
        request.Content = content;
        request.Headers.UserAgent.ParseAdd($"hashi-pulse/{Version}");

        var response = await client.SendAsync(request, cancellationToken);
        if (!response.IsSuccessStatusCode)
        {
            throw new Exception($"unexpected heartbeat status {(int)response.StatusCode} ({response.ReasonPhrase})");
        }

        Console.WriteLine($"heartbeat accepted ({payload.PrivateIpv4Candidates.Count} private IPv4 candidate(s), {payload.PrivateIpv6Candidates.Count} private IPv6 candidate(s))");
    }
}
