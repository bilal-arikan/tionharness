package agent

import "github.com/bilal-arikan/tionharness/internal/mcp"

// searchTool is the namespaced codebase-memory search tool the recovery-step
// tests reference; built through the same helper the registry uses (see the
// twin in internal/mcp/repair) so a namespace-format change breaks tests loudly.
var searchTool = mcp.NamespaceTool("codebase-memory-mcp", "search_code")

// tionharnessCwd is a session working directory whose derived project id is a
// plausible codebase-memory project (twin of the repair package fixture).
const tionharnessCwd = `C:\Users\user\Desktop\Projects\TionHarness`
