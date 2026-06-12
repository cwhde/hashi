# H-045: Pulse Dockerfile Build Cache Key References Non-Existent go.sum

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.4, §17.6

**Status:** Fixed
**Branch:** h/backend-quality

## Description

The CI workflow `ci.yml` and the Pulse Docker build workflow reference `go.sum` in their cache key hashes:

```yaml
# ci.yml line ~253
key: go-pulse-${{ runner.os }}-${{ hashFiles('agents/pulse/go.mod', 'agents/pulse/go.sum') }}
```

However, the Pulse agent (`agents/pulse/`) has **zero external dependencies** — its `go.mod` contains no `require` block, and there is **no `go.sum` file** in the repository. The `.gitignore` at `agents/pulse/.gitignore` ignores only the `hashi-pulse` binary.

When `hashFiles` cannot find `go.sum`, the behavior depends on the Gitea Actions runner implementation:
1. It may produce an empty hash, resulting in cache misses on every run
2. It may fail silently, never producing a valid cache key
3. It may warn but produce a degenerate key

In all cases, the Go module/build caches are effectively never used or are produced with inconsistent keys, negating the caching optimization.

## Evidence

```yaml
# ci.yml — lines 250-255
key: go-pulse-${{ runner.os }}-${{ hashFiles('agents/pulse/go.mod', 'agents/pulse/go.sum') }}
```

```yaml
# docker-build-pulse.yml — lines 37-40
key: go-pulse-${{ runner.os }}-${{ hashFiles('agents/pulse/go.mod', 'agents/pulse/go.sum') }}
```

```go
// agents/pulse/go.mod — empty require block
module github.com/hashi-app/hashi/agents/pulse
go 1.22
```

No `agents/pulse/go.sum` file exists in the repository.

## Expected Outcome

The cache key should use only files that actually exist. Since the Pulse agent has no external dependencies, the `go.mod` file hash alone is sufficient:

```yaml
key: go-pulse-${{ runner.os }}-${{ hashFiles('agents/pulse/go.mod') }}
```

## Fix Guidance

1. Remove `go.sum` from the `hashFiles` calls in both `ci.yml` and `docker-build-pulse.yml`.
2. Ensure `go.sum` is also not referenced in restore keys.

## Acceptance Criteria

- [ ] CI workflow cache key uses only existing files
- [ ] Cache hit/miss behavior is predictable and logs are clear
- [ ] `go.sum` is not referenced in any cache key where it doesn't exist
- [ ] Builds still succeed if `go.sum` is later added (cache key will change naturally)
