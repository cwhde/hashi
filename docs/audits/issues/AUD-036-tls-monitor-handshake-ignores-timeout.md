# AUD-036: TLS monitor handshake ignores the configured timeout

Priority: P2 — one stalled TLS endpoint can stop all periodic monitoring

Category: Monitoring correctness / resource exhaustion

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §18.2 specifies TLS certificate-expiry checks; §18.3 requires endpoint state to reflect whether required checks pass. §24 lists the monitor timeout setting.

## Finding

The TLS monitor passes a linked timeout token to `TcpClient.ConnectAsync`, but calls `SslStream.AuthenticateAsClientAsync` without that token or an explicit timeout. A server can accept the TCP connection and then withhold its TLS handshake response. The check then waits beyond the configured timeout. `MonitorCheckWorker` awaits endpoint checks sequentially in one background loop, so one such endpoint can block checks and state updates for all later endpoints indefinitely.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs`: `CheckTlsAsync` creates a linked cancellation source and applies `CancelAfter(timeout)`; the `SslStream.AuthenticateAsClientAsync(host, null, SslProtocols.None, false)` call does not receive the linked token.
- `MonitorCheckWorker.RunChecksAsync` awaits each endpoint check inside a sequential `foreach` loop.
- The worker only schedules its next iteration after `RunChecksAsync` returns.

## Expected outcome

Bound the full TLS probe, including handshake and certificate inspection, by the configured per-check timeout. A timeout should produce a failed check and allow the worker to continue with the remaining endpoints.

## Acceptance criteria

- TLS handshake uses an overload accepting the linked cancellation token, or an equivalent bounded operation.
- A peer that accepts TCP but does not complete a TLS handshake returns failure within the configured timeout.
- One timed-out TLS endpoint does not prevent subsequent endpoints from being checked.
- Cancellation during application shutdown remains distinguishable from an endpoint timeout.
