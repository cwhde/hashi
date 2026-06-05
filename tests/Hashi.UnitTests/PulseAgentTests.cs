using System;
using System.Collections.Generic;
using System.Net;
using System.Text.Json;
using Xunit;
using PulseAgent = Hashi.Pulse.Program;

namespace Hashi.UnitTests;

public class PulseAgentTests
{
    [Fact]
    public void TestDurationEnvFallback()
    {
        var result = PulseAgent.ParseDuration("not-a-duration", TimeSpan.FromSeconds(42));
        Assert.Equal(TimeSpan.FromSeconds(42), result);

        Assert.Equal(TimeSpan.FromSeconds(60), PulseAgent.ParseDuration("60s", TimeSpan.FromMinutes(1)));
        Assert.Equal(TimeSpan.FromMinutes(1), PulseAgent.ParseDuration("1m", TimeSpan.FromMinutes(2)));
        Assert.Equal(TimeSpan.FromHours(2), PulseAgent.ParseDuration("2h", TimeSpan.FromMinutes(2)));
        Assert.Equal(TimeSpan.FromMilliseconds(500), PulseAgent.ParseDuration("500ms", TimeSpan.FromMinutes(2)));
        Assert.Equal(TimeSpan.FromSeconds(10), PulseAgent.ParseDuration("10", TimeSpan.FromMinutes(2)));
    }

    [Fact]
    public void TestIsPrivateIPv6()
    {
        var privateIp = IPAddress.Parse("fd00::1");
        Assert.True(PulseAgent.IsPrivateIPv6(privateIp));

        var privateIp2 = IPAddress.Parse("fc00::1");
        Assert.True(PulseAgent.IsPrivateIPv6(privateIp2));

        var publicIp = IPAddress.Parse("2001:db8::1");
        Assert.False(PulseAgent.IsPrivateIPv6(publicIp));

        var linkLocalIp = IPAddress.Parse("fe80::1");
        Assert.False(PulseAgent.IsPrivateIPv6(linkLocalIp));
    }

    [Fact]
    public void TestIsPrivateIPv4()
    {
        Assert.True(PulseAgent.IsPrivateIPv4(IPAddress.Parse("10.0.0.5")));
        Assert.True(PulseAgent.IsPrivateIPv4(IPAddress.Parse("172.16.0.1")));
        Assert.True(PulseAgent.IsPrivateIPv4(IPAddress.Parse("172.31.255.255")));
        Assert.True(PulseAgent.IsPrivateIPv4(IPAddress.Parse("192.168.1.100")));

        Assert.False(PulseAgent.IsPrivateIPv4(IPAddress.Parse("8.8.8.8")));
        Assert.False(PulseAgent.IsPrivateIPv4(IPAddress.Parse("127.0.0.1")));
        Assert.False(PulseAgent.IsPrivateIPv4(IPAddress.Parse("172.15.255.255")));
        Assert.False(PulseAgent.IsPrivateIPv4(IPAddress.Parse("172.32.0.1")));
        Assert.False(PulseAgent.IsPrivateIPv4(IPAddress.Parse("169.254.1.1")));
    }

    [Fact]
    public void TestHeartbeatPayloadSerialization()
    {
        var payload = new Hashi.Pulse.PulseHeartbeatAuthRequest
        {
            Token = "test-token",
            Version = "0.1.0",
            Hostname = "test-host",
            PrivateIpv4Candidates = new List<string> { "10.0.0.5" },
            PrivateIpv6Candidates = new List<string> { "fd00::5" },
            SelectedInterface = "eth0",
            SelectedIp = "10.0.0.5",
            Timestamp = DateTimeOffset.Parse("2026-06-01T12:00:00Z"),
            Docker = new Hashi.Pulse.PulseDockerMetadataRequest
            {
                ContainerId = "abc123def456",
                Image = "hashi-pulse:latest",
                NetworkMode = "bridge"
            }
        };

        var json = JsonSerializer.Serialize(payload, Hashi.Pulse.PulseJsonContext.Default.PulseHeartbeatAuthRequest);

        var deserialized = JsonSerializer.Deserialize(json, Hashi.Pulse.PulseJsonContext.Default.PulseHeartbeatAuthRequest);

        Assert.NotNull(deserialized);
        Assert.Equal("test-token", deserialized.Token);
        Assert.Equal("0.1.0", deserialized.Version);
        Assert.Equal("test-host", deserialized.Hostname);
        Assert.Single(deserialized.PrivateIpv4Candidates);
        Assert.Equal("10.0.0.5", deserialized.PrivateIpv4Candidates[0]);
        Assert.Single(deserialized.PrivateIpv6Candidates);
        Assert.Equal("fd00::5", deserialized.PrivateIpv6Candidates[0]);
        Assert.Equal("eth0", deserialized.SelectedInterface);
        Assert.Equal("10.0.0.5", deserialized.SelectedIp);
        Assert.Equal(DateTimeOffset.Parse("2026-06-01T12:00:00Z"), deserialized.Timestamp);

        Assert.NotNull(deserialized.Docker);
        Assert.Equal("abc123def456", deserialized.Docker.ContainerId);
        Assert.Equal("hashi-pulse:latest", deserialized.Docker.Image);
        Assert.Equal("bridge", deserialized.Docker.NetworkMode);
    }

    [Fact]
    public void TestLoadConfigFromArguments()
    {
        var args = new[]
        {
            "--api", "https://hashi.example.com/",
            "-agent-id", "test-agent-id",
            "--token", "test-token",
            "--interface", "eth1",
            "--interval", "30s",
            "--timeout", "5s",
            "--once"
        };

        var (config, err) = PulseAgent.LoadConfig(args);

        Assert.Null(err);
        Assert.Equal("https://hashi.example.com", config.ApiUrl);
        Assert.Equal("test-agent-id", config.AgentId);
        Assert.Equal("test-token", config.Token);
        Assert.Equal("eth1", config.SelectedInterface);
        Assert.Equal(TimeSpan.FromSeconds(30), config.Interval);
        Assert.Equal(TimeSpan.FromSeconds(5), config.Timeout);
        Assert.True(config.Once);
    }

    [Fact]
    public void TestLoadConfigFromEnvironmentAndArgs()
    {
        Environment.SetEnvironmentVariable("HASHI_PULSE_API", "https://env.example.com");
        Environment.SetEnvironmentVariable("HASHI_PULSE_AGENT_ID", "env-id");
        Environment.SetEnvironmentVariable("HASHI_PULSE_TOKEN", "env-token");
        Environment.SetEnvironmentVariable("HASHI_PULSE_INTERFACE", "eth0");
        Environment.SetEnvironmentVariable("HASHI_PULSE_INTERVAL", "45s");
        Environment.SetEnvironmentVariable("HASHI_PULSE_TIMEOUT", "15s");
        Environment.SetEnvironmentVariable("HASHI_PULSE_ONCE", "0");

        try
        {
            var (config, err) = PulseAgent.LoadConfig(Array.Empty<string>());
            Assert.Null(err);
            Assert.Equal("https://env.example.com", config.ApiUrl);
            Assert.Equal("env-id", config.AgentId);
            Assert.Equal("env-token", config.Token);
            Assert.Equal("eth0", config.SelectedInterface);
            Assert.Equal(TimeSpan.FromSeconds(45), config.Interval);
            Assert.Equal(TimeSpan.FromSeconds(15), config.Timeout);
            Assert.False(config.Once);

            var (config2, err2) = PulseAgent.LoadConfig(new[] { "--token", "override-token", "--once" });
            Assert.Null(err2);
            Assert.Equal("https://env.example.com", config2.ApiUrl);
            Assert.Equal("env-id", config2.AgentId);
            Assert.Equal("override-token", config2.Token);
            Assert.True(config2.Once);
        }
        finally
        {
            Environment.SetEnvironmentVariable("HASHI_PULSE_API", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_AGENT_ID", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_TOKEN", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_INTERFACE", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_INTERVAL", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_TIMEOUT", null);
            Environment.SetEnvironmentVariable("HASHI_PULSE_ONCE", null);
        }
    }
}
