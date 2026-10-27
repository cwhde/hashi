//go:build dev

package app

// devBuild enables what local development and the E2E suite need and production must never have:
//   - HASHI_DEV_WEBAUTHN_ORIGIN (e.g. http://localhost:5173) binds passkeys to RP ID "localhost" at that origin,
//     because Hashi has no HTTPS domain before S5 (02-setup-auth.md §1);
//   - setup steps from later slices count as done, so the wizard can complete.
const devBuild = true
