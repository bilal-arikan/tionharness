package db

// MCP transport kinds.
const (
	MCPTransportStdio = "stdio"
	MCPTransportSSE   = "sse"
	MCPTransportHTTP  = "http"
)

// MCPServer is a configured Model Context Protocol server. A stdio server is
// launched as a subprocess (Command + Args); an http server (Streamable HTTP) is
// reached at URL. EnvConfig holds extra environment variables (stdio) and
// HeadersConfig extra request headers (http, e.g. Authorization) as JSON objects.
type MCPServer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Transport     string `json:"transport"`     // stdio | http (sse deprecated/unsupported)
	Command       string `json:"command"`       // stdio executable
	Args          string `json:"args"`          // JSON array of args
	URL           string `json:"url"`           // http endpoint
	EnvConfig     string `json:"envConfig"`     // JSON object of env vars (stdio)
	HeadersConfig string `json:"headersConfig"` // JSON object of request headers (http)
	Enabled       bool   `json:"enabled"`
	Scope         string `json:"scope"`               // shared | scoped
	CreatedBy     string `json:"createdBy,omitempty"` // "" = user-defined (protected); agent id = agent-created
	CreatedAt     int64  `json:"createdAt"`
}
