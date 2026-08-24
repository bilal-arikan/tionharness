// Extension -> syntax-highlight language for text artifacts. Split out so
// TextFileArtifact.tsx exports only components (fast refresh).
const TEXT_EXT: Record<string, string> = {
  md: 'markdown',
  markdown: 'markdown',
  txt: '',
  log: '',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  csv: '',
  toml: 'toml',
  ts: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  go: 'go',
  py: 'python',
  sh: 'bash',
  sql: 'sql',
}

// Max bytes rendered inline; larger files stay a download-only card so the
// artifacts screen can't be wedged by a huge log.

export function textFileLang(sourcePath: string): string | undefined {
  const ext = sourcePath.split('.').pop()?.toLowerCase() ?? ''
  return ext in TEXT_EXT ? TEXT_EXT[ext] : undefined
}

// TextFileArtifact fetches a stored text file and renders it inline: markdown
// files as prose, everything else as a syntax-highlighted block.
