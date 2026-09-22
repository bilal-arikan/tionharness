package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// zvecGrepManifestDoc is the subset of .zvec-grep/manifest.json the index
// manager reads. zvec-grep writes more than this; only the fields that drive a
// lifecycle decision are declared, so an upstream addition cannot break parsing.
type zvecGrepManifestDoc struct {
	Embedding struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"embedding"`
	// UpdatedTime is epoch MILLIseconds (zvec-grep writes JS timestamps).
	UpdatedTime int64 `json:"updatedTime"`
}

// zvecGrepManifestInfo is a parsed manifest in the terms the manager reasons in.
type zvecGrepManifestInfo struct {
	// Embedding is "<provider>/<model>", matching the form `zg index --embedding`
	// takes and the form zvecGrepEmbedding() returns, so the two are comparable
	// without normalising at every call site.
	Embedding string
	UpdatedAt time.Time
}

// readZvecGrepManifest reads the manifest of the index at root.
//
// Errors are RETURNED, not swallowed: an unreadable or malformed manifest next
// to an existing store means the manager does not know what that store was built
// with, and the caller must surface that rather than assume the index is fine.
func readZvecGrepManifest(root string) (zvecGrepManifestInfo, error) {
	path := filepath.Join(root, zvecGrepIndexDir, zvecGrepManifest)
	data, err := os.ReadFile(path)
	if err != nil {
		return zvecGrepManifestInfo{}, err
	}
	var doc zvecGrepManifestDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return zvecGrepManifestInfo{}, fmt.Errorf("%s okunamadı: %w", path, err)
	}
	info := zvecGrepManifestInfo{}
	provider := strings.TrimSpace(doc.Embedding.Provider)
	model := strings.TrimSpace(doc.Embedding.Model)
	if provider != "" && model != "" {
		info.Embedding = provider + "/" + model
	}
	if doc.UpdatedTime > 0 {
		info.UpdatedAt = time.UnixMilli(doc.UpdatedTime)
	}
	return info, nil
}
