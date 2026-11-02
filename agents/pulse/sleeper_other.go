//go:build !(linux && (amd64 || arm64))

package main

// No sleeper here (a developer's machine): the agent stays in memory between beats.
var sleeperImage []byte

const sysMemfdCreate = 0
