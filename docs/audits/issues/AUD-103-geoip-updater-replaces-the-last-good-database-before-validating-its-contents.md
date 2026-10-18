# AUD-103: GeoIP updater replaces the last good database before validating its contents

Priority: P2

Category: Reliability / GeoIP updates

Status: open; confirmed by manual source trace.

## Finding

The downloader accepts the matching archive entry as arbitrary bytes, calculates its hash and returns success. UpdateAsync atomically replaces the live file and deletes the backup before opening or validating it as the expected MMDB edition. A corrupt but extractable matching entry destroys the last good database and reports Succeeded; Reload only discovers the error on later reads. ResponseHeadersRead also leaves body/decompression without an explicit total read deadline or expanded-size bound. Concurrent manual/worker updates have no exclusion lock.

## Evidence

src/Hashi.Infrastructure/Platform/GeoIpUpdateService.cs:210-252,306-316,334-390. Atomic rename provides write atomicity but does not establish semantic validity or preserve a usable rollback file.

## Expected outcome and acceptance criteria

Validate metadata/type and representative reads before replacement, retain the previous usable edition through activation, serialize updates, and bound download/decompression duration and size. An invalid replacement must leave existing rules operational and report failure.
