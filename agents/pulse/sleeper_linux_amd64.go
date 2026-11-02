package main

import _ "embed"

//go:embed sleeper/sleeper_amd64.bin
var sleeperImage []byte

const sysMemfdCreate = 319
