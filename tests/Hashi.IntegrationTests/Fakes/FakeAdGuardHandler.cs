using System.Collections.Concurrent;
using System.Net;
using System.Text.Json;

namespace Hashi.IntegrationTests.Fakes;

public sealed class FakeAdGuardHandler : HttpMessageHandler
{
    private readonly ConcurrentDictionary<string, FakeRewrite> _rewrites = new(StringComparer.OrdinalIgnoreCase);
    private int _rewriteCounter;

    public int DeleteCalls { get; private set; }
    public int CreateCalls { get; private set; }

    public FakeAdGuardHandler()
    {
        AddRewrite("existing.example.com", "10.0.0.1");
    }

    public string DefaultBaseUrl => "http://adguard.test";

    public void AddRewrite(string domain, string answer, string? id = null)
    {
        id ??= Interlocked.Increment(ref _rewriteCounter).ToString();
        _rewrites[id] = new FakeRewrite(id, domain, answer);
    }

    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        var path = request.RequestUri?.AbsolutePath.Trim('/') ?? string.Empty;

        if (path.EndsWith("/control/rewrite/list", StringComparison.Ordinal) && request.Method == HttpMethod.Get)
        {
            return Task.FromResult(JsonResponse(HttpStatusCode.OK, new
            {
                rewrites = _rewrites.Values.Select(r => new { id = r.Id, domain = r.Domain, answer = r.Answer }).ToArray(),
            }));
        }

        if (path.EndsWith("/control/rewrite/add", StringComparison.Ordinal) && request.Method == HttpMethod.Post)
        {
            CreateCalls++;
            return Task.FromResult(JsonResponse(HttpStatusCode.OK, new { }));
        }

        if (path.EndsWith("/control/rewrite/delete", StringComparison.Ordinal) && request.Method == HttpMethod.Post)
        {
            DeleteCalls++;
            return Task.FromResult(JsonResponse(HttpStatusCode.OK, new { }));
        }

        if (path.EndsWith("/control/status", StringComparison.Ordinal))
        {
            return Task.FromResult(JsonResponse(HttpStatusCode.OK, new { dns_addresses = new[] { "10.0.0.53" } }));
        }

        return Task.FromResult(new HttpResponseMessage(HttpStatusCode.NotFound));
    }

    private static HttpResponseMessage JsonResponse(HttpStatusCode status, object payload)
        => new(status)
        {
            Content = new StringContent(JsonSerializer.Serialize(payload), System.Text.Encoding.UTF8, "application/json"),
        };

    private sealed class FakeRewrite(string id, string domain, string answer)
    {
        public string Id { get; } = id;
        public string Domain { get; } = domain;
        public string Answer { get; } = answer;
    }
}
