using System.Text.Json.Serialization;

namespace Hashi.Pulse;

public sealed class PulseHeartbeatAuthRequest
{
    [JsonPropertyName("token")]
    public string Token { get; set; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; set; } = string.Empty;

    [JsonPropertyName("hostname")]
    public string Hostname { get; set; } = string.Empty;

    [JsonPropertyName("privateIpv4Candidates")]
    public List<string> PrivateIpv4Candidates { get; set; } = [];

    [JsonPropertyName("privateIpv6Candidates")]
    public List<string> PrivateIpv6Candidates { get; set; } = [];

    [JsonPropertyName("selectedInterface")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? SelectedInterface { get; set; }

    [JsonPropertyName("selectedIp")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? SelectedIp { get; set; }

    [JsonPropertyName("timestamp")]
    public DateTimeOffset Timestamp { get; set; }

    [JsonPropertyName("docker")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public PulseDockerMetadataRequest? Docker { get; set; }
}

public sealed class PulseDockerMetadataRequest
{
    [JsonPropertyName("containerId")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? ContainerId { get; set; }

    [JsonPropertyName("image")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Image { get; set; }

    [JsonPropertyName("networkMode")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? NetworkMode { get; set; }
}

[JsonSerializable(typeof(PulseHeartbeatAuthRequest))]
[JsonSerializable(typeof(PulseDockerMetadataRequest))]
[JsonSourceGenerationOptions(PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase)]
internal partial class PulseJsonContext : JsonSerializerContext
{
}
