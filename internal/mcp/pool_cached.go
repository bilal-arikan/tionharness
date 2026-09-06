package mcp

// CatalogCached is the no-dial twin of Catalog: it returns the tool list of
// every server whose pooled connection is already alive and listed, and
// silently skips the rest. It never starts a process, never waits on an
// initialize handshake and never calls tools/list, so a read-only surface
// (the session info panel, a context preview) can build a registry without
// paying a cold server's DefaultDialTimeout per server. cfgByServer carries
// every config handed in, connected or not, so a caller that only needs the
// routing map keeps the same shape Catalog produces; skipped names the servers
// that had no live connection.
func (p *Pool) CatalogCached(cfgs []ServerConfig) (entries []CatalogEntry, cfgByServer map[string]ServerConfig, skipped []string) {
	cfgByServer = map[string]ServerConfig{}
	for _, cfg := range cfgs {
		name, _, _ := SplitNamespaced(NamespaceTool(cfg.Name, "x"))
		cfgByServer[name] = cfg
		e := p.entry(scopedEntryKey(cfg.ScopeKey, name))
		e.mu.Lock()
		live := e.client != nil && e.fp == configFingerprint(cfg) && e.client.Alive() && e.listed
		var list []Tool
		if live {
			list = append([]Tool(nil), e.tools...)
		}
		e.mu.Unlock()
		if !live {
			skipped = append(skipped, cfg.Name)
			continue
		}
		for _, t := range list {
			entries = append(entries, CatalogEntry{
				Server:         cfg.Name,
				NamespacedName: NamespaceTool(cfg.Name, t.Name),
				Tool:           t,
			})
		}
	}
	return entries, cfgByServer, skipped
}
