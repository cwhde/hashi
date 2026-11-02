package main

import _ "embed"

//go:embed sleeper/sleeper_arm64.bin
var sleeperImage []byte

const sysMemfdCreate = 279
