# AUD-067: Blocklist response body is read without the configured timeout

Priority: P2

Category: Reliability / external downloads

Status: open; confirmed by manual source tracing.

## Finding

SocketsBlocklistHttpTransport applies HttpClient.Timeout to SendAsync with ResponseHeadersRead, then reads the body using only the caller cancellation token. After headers arrive, the configured timeout no longer bounds body reads. A feed that stalls or drips bytes can hold RefreshDueSourcesAsync indefinitely and prevent other source maintenance. The byte limit does not impose a time limit.

## Evidence

src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:1054-1077,1080-1106

## Expected outcome

Use one linked deadline across DNS/connect/headers/body and retain last-known-good state on expiration.

## Acceptance criteria

A source that sends headers then stalls or drips bytes terminates at the configured total deadline, preserves entries and allows later sources/jobs to run.

No live provider state was modified to establish this finding.

Primary reference: [Microsoft behavior documentation](https://learn.microsoft.com/en-us/dotnet/api/system.net.http.httpcompletionoption).
