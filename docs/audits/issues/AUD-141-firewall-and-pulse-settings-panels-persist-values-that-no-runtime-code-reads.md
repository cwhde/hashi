# AUD-141: Firewall and Pulse settings panels persist values that no runtime code reads

Priority: P1

Category: Missing implementation / settings

Status: open; confirmed by manual source trace.

## Finding

The settings page exposes trusted CIDRs, port confirmation, persistence mode, NetBird enablement, heartbeat interval and stale threshold through category JSON. The category endpoint serializes these into SettingsCategoriesJson. Repository-wide production references to that field only implement category storage/readback; firewall/Pulse runtime services never consume the stored categories. Saving reports success but does not change the advertised operational policy. Entity-specific host settings existing elsewhere do not make these global settings effective.

## Evidence

web/src/routes/(admin)/settings/+page.svelte firewall/pulse forms and save functions at479-503; src/Hashi.Api/Features/Setup/SettingsEndpoints.cs:261-303; AppSettingsEntity.SettingsCategoriesJson and no runtime consumers beyond category persistence.

## Expected outcome and acceptance criteria

Replace inert JSON panels with validated typed settings connected to their actual runtime policy, or clearly remove unsupported controls pending implementation. Demonstrate each visible control changing the rendered host policy or effective heartbeat/stale behavior.
