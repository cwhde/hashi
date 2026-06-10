namespace Hashi.Api.Features.Platform;

using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

public static class ErrorEndpoints
{
    public static IEndpointRouteBuilder MapErrorEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/error/{status:int}", (int status) =>
        {
            var title = status switch
            {
                500 => "Internal Server Error",
                502 => "Bad Gateway",
                503 => "Service Unavailable",
                504 => "Gateway Timeout",
                _ => "Server Error"
            };

            var description = status switch
            {
                500 => "The server encountered an internal error or misconfiguration and was unable to complete your request.",
                502 => "The server received an invalid response from an upstream server.",
                503 => "The server is temporarily unable to service your request due to maintenance downtime or capacity problems.",
                504 => "The upstream server failed to send a request in a timely manner.",
                _ => "An unexpected error occurred while processing your request."
            };

            var html = $$"""
                <!DOCTYPE html>
                <html lang="en">
                <head>
                    <meta charset="UTF-8">
                    <meta name="viewport" content="width=device-width, initial-scale=1.0">
                    <title>{{status}} - {{title}}</title>
                    <style>
                        :root {
                            --bg: #0b0d11;
                            --panel: #151922;
                            --text: #f3f4f6;
                            --text-muted: #9ca3af;
                            --primary: #6366f1;
                            --accent: #ef4444;
                            --border: #272d3d;
                        }
                        body {
                            background-color: var(--bg);
                            color: var(--text);
                            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
                            margin: 0;
                            display: flex;
                            align-items: center;
                            justify-content: center;
                            min-height: 100vh;
                            box-sizing: border-box;
                            padding: 2rem;
                        }
                        .container {
                            max-width: 480px;
                            width: 100%;
                            background-color: var(--panel);
                            border: 1px solid var(--border);
                            border-radius: 12px;
                            padding: 2.5rem;
                            box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.3), 0 8px 10px -6px rgba(0, 0, 0, 0.3);
                            text-align: center;
                            animation: fadeIn 0.5s ease-out;
                        }
                        @keyframes fadeIn {
                            from { opacity: 0; transform: translateY(10px); }
                            to { opacity: 1; transform: translateY(0); }
                        }
                        .error-code {
                            font-size: 5rem;
                            font-weight: 800;
                            color: var(--accent);
                            margin: 0;
                            line-height: 1;
                            letter-spacing: -0.05em;
                        }
                        h1 {
                            font-size: 1.5rem;
                            font-weight: 700;
                            margin-top: 1rem;
                            margin-bottom: 0.75rem;
                        }
                        p {
                            color: var(--text-muted);
                            font-size: 0.95rem;
                            line-height: 1.6;
                            margin-bottom: 2rem;
                        }
                        .divider {
                            height: 1px;
                            background-color: var(--border);
                            margin: 1.5rem 0;
                        }
                        .footer {
                            font-size: 0.8rem;
                            color: var(--text-muted);
                        }
                    </style>
                </head>
                <body>
                    <div class="container">
                        <div class="error-code">{{status}}</div>
                        <h1>{{title}}</h1>
                        <p>{{description}}</p>
                        <div class="divider"></div>
                        <div class="footer">Hashi Security Gateway</div>
                    </div>
                </body>
                </html>
                """;

            return Results.Content(html, "text/html", System.Text.Encoding.UTF8, status);
        });

        return app;
    }
}
