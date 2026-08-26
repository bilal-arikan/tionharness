export interface QuickstartTab {
  id: string
  label: string
  /** Rendered verbatim inside a <pre>. Keep lines short enough to avoid wrapping. */
  code: string
}

/**
 * Until releases exist, the only honest install path is "build from source".
 * When site.config.downloads is filled in, add a first tab that just downloads
 * the binary and keep these as the from-source fallback.
 */
export const quickstartTabs: QuickstartTab[] = [
  {
    id: 'windows',
    label: 'Windows',
    code: `# Prerequisites: Go 1.26+, Node 20+, and the claude CLI already logged in.
git clone <REPO_URL> tionharness
cd tionharness

# Builds the UI into internal/web/dist, then embeds it into one exe.
.\\scripts\\build.ps1

$env:TIONHARNESS_ADDR="127.0.0.1:8095"
.\\tionharness.exe            # open http://127.0.0.1:8095`,
  },
  {
    id: 'linux',
    label: 'Linux',
    code: `# Prerequisites: Go 1.26+, Node 20+, and the claude CLI already logged in.
git clone <REPO_URL> tionharness
cd tionharness

cd frontend && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o tionharness ./cmd/tionharness

TIONHARNESS_ADDR=127.0.0.1:8095 ./tionharness`,
  },
  {
    id: 'macos',
    label: 'macOS',
    code: `# Prerequisites: Go 1.26+, Node 20+, and the claude CLI already logged in.
git clone <REPO_URL> tionharness
cd tionharness

cd frontend && npm ci && npm run build && cd ..
go build -trimpath -ldflags "-s -w" -o tionharness ./cmd/tionharness

TIONHARNESS_ADDR=127.0.0.1:8095 ./tionharness`,
  },
]

export const devModeSnippet = `# One command for backend + frontend with hot reload
.\\scripts\\dev.ps1              # LAN-visible, opens the browser
.\\scripts\\dev.ps1 -Loopback    # 127.0.0.1 only, no firewall prompt`
