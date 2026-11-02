#!/bin/sh
# Builds the committed sleeper binaries with one pinned compiler, for both architectures, from any machine with Docker.
set -eu
cd "$(dirname "$0")"
docker run --rm -v "$PWD:/w" -w /w alpine:3.24 sh -ec '
apk add -q clang lld
for a in amd64:x86_64 arm64:aarch64; do
	clang --target="${a#*:}-linux-musl" -Os -static -nostdlib -ffreestanding -fno-stack-protector -fno-asynchronous-unwind-tables \
		-fuse-ld=lld -Wl,--build-id=none,-z,noseparate-code,-s sleeper.c -o "sleeper_${a%:*}.bin"
done'
