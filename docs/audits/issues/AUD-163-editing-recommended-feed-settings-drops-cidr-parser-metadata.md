# AUD-163: Editing recommended feed settings drops CIDR parser metadata

Priority: P2

Category: Wrong implementation / blocklist parsing

Status: open; confirmed by manual source trace.

## Finding and evidence

Recommended DShield metadata records `valueColumnIndex=0` and `cidrPrefixColumnIndex=2`. UpdateAsync replaces the entire parser object through MergeParserMetadata, which writes only request CSV column and JSON fields. The request has no CIDR-prefix-column field. A routine source edit therefore deletes the prefix-column interpretation; subsequent parser reads cannot reconstruct the feed's intended network ranges and can reduce protection to individual addresses or reject rows. Existing parser-specific settings should not disappear on an unrelated enabled/mode/name edit.

Evidence: `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:69-70,479-492,1150-1158,1408`.

## Expected outcome and acceptance criteria

Preserve unsupported/unchanged parser metadata, expose all required format options in a typed model and validate source edits against representative content. Editing a DShield source without changing its parser must yield the same normalized CIDR entries before and after the update.
