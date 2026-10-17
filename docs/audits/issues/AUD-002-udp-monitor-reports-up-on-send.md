# AUD-002 - UDP monitoring reports healthy after sending without a response

Priority: Medium (P2)

Category: Bug / monitoring correctness

Specification reference: `docs/implementation-spec/hashi-v2-implementation-spec.md`, sections 18.2 and 18.3 (UDP basic response checks; Up means required checks pass).

## Problem

The UDP monitor probe sends an empty datagram and returns `true` when the local socket accepts the send. UDP send success does not establish that the remote host is reachable or that the target service responded, so endpoints can be marked Up while the service is stopped or the destination is unreachable. The current “basic liveness check” comment describes a weaker behavior than the specification requires.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs`, `DefaultMonitorNetworkProbe.ProbeUdpAsync`, calls `UdpClient.SendAsync([], 0)` and returns `true` without receiving a datagram or validating a protocol response.
- `MonitorCheckWorker.CheckEndpointStatusAsync` maps that boolean directly to `up` or `down`.
- The specification lists “UDP basic response checks where configured,” which requires observing a response.

## Expected outcome

UDP checks use a configurable probe payload and a bounded response/read policy appropriate to the selected protocol. A successful local send alone is never reported as a healthy remote check. If a protocol cannot be probed generically, the UI and API should call it a send-only check and avoid presenting it as service health.

## Acceptance criteria

- Define supported UDP probe/response behavior and configuration, with a safe timeout and response-size limit.
- Return Up only after receiving and, where applicable, validating an expected response.
- Represent send-only behavior separately from healthy service response if retained.
- Add unit tests for response, timeout/no response, and malformed or unexpected response.
