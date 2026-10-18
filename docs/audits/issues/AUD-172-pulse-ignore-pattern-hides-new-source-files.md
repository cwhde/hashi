# AUD-172: Pulse ignore pattern hides new source files

Priority: P3

Category: Development reliability / repository configuration

Status: open; independently checked with git check-ignore.

## Finding and evidence

`agents/pulse/.gitignore` contains the unanchored name `hashi-pulse`, intended to ignore the compiled binary. Git also matches the `cmd/hashi-pulse/` source directory, so new source/test files under that directory are ignored by default. Existing tracked main.go files remain tracked, but ordinary ignored-file-aware searches omit them and newly created implementation files can be left out of commits accidentally. This audit used the complete Git-tracked inventory and explicitly read those existing files.

`git check-ignore -v agents/pulse/cmd/hashi-pulse/audit-example-new-file.go` identifies the pattern; no example file was created.

## Expected outcome and acceptance criteria

Anchor the binary ignore path appropriately. A new source file in cmd/hashi-pulse must be visible to normal status/search/add workflows, while the root compiled executable remains ignored. Preserve already tracked source.
