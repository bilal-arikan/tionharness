package market

import "embed"

// bundledFS holds the workspace-template packs that ship embedded in the binary.
// They form the read-only SourceBundled tier (lowest priority) so a fresh install
// always has workspace templates available in the market and the create-workspace
// picker, even before any remote registry is added. Higher tiers (global, remote)
// override a bundled pack of the same id.
//
//go:embed defaults/*.swarmpack.json
var bundledFS embed.FS

// bundledDir is the directory inside bundledFS holding the pack files.
const bundledDir = "defaults"
