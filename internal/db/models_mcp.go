package db

// MCP transport kinds.
const (
	MCPTransportStdio = "stdio"
	MCPTransportSSE   = "sse"
	MCPTransportHTTP  = "http"
)

// MCPServer is a configured Model Context Protocol server. A stdio server is
// launched as a subprocess (Command + Args); sse/http servers are reached at
// URL. EnvConfig holds extra environment variables as a JSON object.
type MCPServer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Transport string `json:"transport"` // stdio | sse | http
	Command   string `json:"command"`   // stdio executable
	Args      string `json:"args"`      // JSON array of args
	URL       string `json:"url"`       // sse/http endpoint
	EnvConfig string `json:"envConfig"` // JSON object of env vars
	Enabled   bool   `json:"enabled"`
	Scope     string `json:"scope"` // shared | scoped
	CreatedAt int64  `json:"createdAt"`
}
