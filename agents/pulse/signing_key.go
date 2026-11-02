package main

// releaseKey is the public half of the key the release job signs agent binaries with (ADR 0003). Its private half is
// the CI secret PULSE_SIGNING_KEY. A new key needs a release built with the old one that carries the new public key.
const releaseKey = "14pzxNP1Zix7jaPkbV0CcMmpYgid2940ESEk36Bwmes="
