package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func validTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func pngConfigOnly(width, height uint32) []byte {
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8], data[9], data[10], data[11], data[12] = 8, 6, 0, 0, 0
	out := append([]byte("\x89PNG\r\n\x1a\n"), 0, 0, 0, 13)
	out = append(out, "IHDR"...)
	out = append(out, data...)
	crcData := append([]byte("IHDR"), data...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(crcData))
	return append(out, crc...)
}

// TestArtifactsContextBlock verifies the system-prompt block lists a session's
// artifacts (id/title/kind) so the agent can revise them with update_artifact,
// and stays empty when there is nothing to surface.
func TestArtifactsContextBlock(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	// Empty session → no block.
	if got := artifactsContextBlock(ctx, database, "sess-1"); got != "" {
		t.Fatalf("expected empty block for no artifacts, got %q", got)
	}
	// Empty session id → no block.
	if got := artifactsContextBlock(ctx, database, ""); got != "" {
		t.Fatalf("expected empty block for empty session id, got %q", got)
	}

	a, err := database.CreateArtifact(ctx, db.Artifact{
		SessionID: "sess-1", AgentID: "agent-1",
		Title: "Plan Doc", Kind: db.ArtifactMarkdown, Content: "# Plan",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	// An artifact in a different session must not leak into sess-1's block.
	if _, err := database.CreateArtifact(ctx, db.Artifact{
		SessionID: "sess-2", Title: "Other", Kind: db.ArtifactCode, Language: "go", Content: "x",
	}); err != nil {
		t.Fatalf("create other artifact: %v", err)
	}

	block := artifactsContextBlock(ctx, database, "sess-1")
	if !strings.Contains(block, a.ID) {
		t.Errorf("block missing artifact id %q:\n%s", a.ID, block)
	}
	if !strings.Contains(block, "Plan Doc") {
		t.Errorf("block missing title:\n%s", block)
	}
	if !strings.Contains(block, "update_artifact") {
		t.Errorf("block should instruct update_artifact:\n%s", block)
	}
	if strings.Contains(block, "Other") {
		t.Errorf("block leaked another session's artifact:\n%s", block)
	}
}

func TestCreateDerivedArtifactConcurrentPreservesOriginal(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	originalRel := "artifacts/SES1/original.png"
	originalPath := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(originalRel))
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	originalBytes := validTestPNG(t, 2, 2)
	if err := os.WriteFile(originalPath, originalBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := wsp.DB.CreateArtifact(ctx, db.Artifact{
		SessionID: "SES1", Title: "Original", Kind: db.ArtifactImage, SourcePath: originalRel, Origin: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	originalHash := sha256.Sum256(originalBytes)
	results := make([]db.Artifact, 2)
	errs := make([]error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		stagedRel := fmt.Sprintf("artifacts/SES1/staged-%d.png", i)
		if err := os.WriteFile(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(stagedRel)), validTestPNG(t, 1, 1), 0o644); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(index int, rel string) {
			defer wg.Done()
			body, _ := json.Marshal(createArtifactReq{SessionID: "SES1", Title: "Original — Düzenleme", Kind: db.ArtifactImage, SourcePath: rel, Origin: "manual", DerivedFromArtifactID: original.ID})
			req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
			rec := httptest.NewRecorder()
			ready.Done()
			<-start
			server.handleCreateArtifact(rec, req)
			if rec.Code != http.StatusOK {
				errs[index] = fmt.Errorf("status %d: %s", rec.Code, rec.Body.String())
				return
			}
			errs[index] = json.Unmarshal(rec.Body.Bytes(), &results[index])
		}(i, stagedRel)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].ID == results[1].ID || results[0].SourcePath == results[1].SourcePath {
		t.Fatalf("collision: %+v %+v", results[0], results[1])
	}
	for _, result := range results {
		if result.DerivedFromArtifactID != original.ID {
			t.Fatalf("wrong parent: %+v", result)
		}
		if _, err := os.Stat(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(result.SourcePath))); err != nil {
			t.Fatalf("derived unreadable: %v", err)
		}
	}
	after, err := wsp.DB.GetArtifact(ctx, original.ID)
	if err != nil || after.ID != original.ID || after.SourcePath != original.SourcePath || after.CreatedAt != original.CreatedAt {
		t.Fatalf("original record changed: %+v, %v", after, err)
	}
	afterBytes, err := os.ReadFile(originalPath)
	if err != nil || int64(len(afterBytes)) != int64(len(originalBytes)) || sha256.Sum256(afterBytes) != originalHash {
		t.Fatalf("original bytes changed: len=%d err=%v", len(afterBytes), err)
	}
}

func TestCreateDerivedArtifactUsesParentSession(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	parentRel := "artifacts/SES-parent/original.png"
	parentPath := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(parentRel))
	if err := os.MkdirAll(filepath.Dir(parentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parentPath, validTestPNG(t, 2, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := wsp.DB.CreateArtifact(ctx, db.Artifact{
		SessionID: "SES-parent", Title: "Original", Kind: db.ArtifactImage, SourcePath: parentRel,
	})
	if err != nil {
		t.Fatal(err)
	}
	stagedRel := "artifacts/staging/spoof.png"
	stagedPath := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(stagedRel))
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, validTestPNG(t, 1, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(createArtifactReq{
		SessionID: "SES-spoofed", Title: "Derived", Kind: db.ArtifactImage,
		SourcePath: stagedRel, DerivedFromArtifactID: parent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	server.handleCreateArtifact(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var derived db.Artifact
	if err := json.Unmarshal(rec.Body.Bytes(), &derived); err != nil {
		t.Fatal(err)
	}
	if derived.SessionID != parent.SessionID {
		t.Fatalf("session = %q, want %q", derived.SessionID, parent.SessionID)
	}
	wantPrefix := "artifacts/" + parent.SessionID + "/"
	if !strings.HasPrefix(derived.SourcePath, wantPrefix) {
		t.Fatalf("source path = %q, want prefix %q", derived.SourcePath, wantPrefix)
	}
	if _, err := os.Stat(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(derived.SourcePath))); err != nil {
		t.Fatalf("derived file: %v", err)
	}
}

func TestCreateDerivedArtifactFailureRemovesStaging(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	nonImage, err := wsp.DB.CreateArtifact(ctx, db.Artifact{SessionID: "SES1", Title: "Text", Kind: db.ArtifactText})
	if err != nil {
		t.Fatal(err)
	}
	missingSource, err := wsp.DB.CreateArtifact(ctx, db.Artifact{
		SessionID: "SES1", Title: "Missing source", Kind: db.ArtifactImage,
		SourcePath: "artifacts/SES1/missing.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		parentID string
		status   int
	}{
		{name: "missing parent", parentID: "missing", status: http.StatusNotFound},
		{name: "non-image parent", parentID: nonImage.ID, status: http.StatusUnprocessableEntity},
		{name: "missing parent source", parentID: missingSource.ID, status: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stagedRel := "artifacts/staging/" + strings.ReplaceAll(test.name, " ", "-") + ".png"
			stagedPath := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(stagedRel))
			if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stagedPath, validTestPNG(t, 1, 1), 0o644); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(createArtifactReq{
				SessionID: "SES1", Title: "Derived", Kind: db.ArtifactImage,
				SourcePath: stagedRel, DerivedFromArtifactID: test.parentID,
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
			rec := httptest.NewRecorder()
			server.handleCreateArtifact(rec, req)
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, test.status, rec.Body.String())
			}
			if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("staging remains: %v", err)
			}
		})
	}
}

func TestHandleArtifactSourceValidation(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	call := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/source", nil)
		req.SetPathValue("id", id)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		server.handleArtifactSource(rec, req)
		return rec
	}
	if rec := call("missing"); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "SOURCE_ARTIFACT_NOT_FOUND") {
		t.Fatalf("missing = %d %s", rec.Code, rec.Body.String())
	}
	nonImage, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "text", Kind: db.ArtifactText})
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(nonImage.ID); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-image status = %d", rec.Code)
	}
	forbidden, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "escape", Kind: db.ArtifactImage, SourcePath: "../secret.png"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(forbidden.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("escape status = %d", rec.Code)
	}
	rel := "artifacts/SES1/source.png"
	path := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := validTestPNG(t, 2, 2)
	if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	image, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "image", Kind: db.ArtifactImage, SourcePath: rel})
	if err != nil {
		t.Fatal(err)
	}
	rec := call(image.ID)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != string(pngBytes) {
		t.Fatalf("valid source = %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	corruptRel := "artifacts/SES1/corrupt.png"
	if err := os.WriteFile(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(corruptRel)), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	corrupt, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "corrupt", Kind: db.ArtifactImage, SourcePath: corruptRel})
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(corrupt.ID); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "IMAGE_HEADER_INVALID") {
		t.Fatalf("corrupt = %d %s", rec.Code, rec.Body.String())
	}
	for name, data := range map[string][]byte{
		"header-junk": append([]byte("\x89PNG\r\n\x1a\n"), []byte("junk")...),
		"truncated":   pngBytes[:len(pngBytes)-8],
	} {
		rel := "artifacts/SES1/" + name + ".png"
		if err := os.WriteFile(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel)), data, 0o644); err != nil {
			t.Fatal(err)
		}
		artifact, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: name, Kind: db.ArtifactImage, SourcePath: rel})
		if err != nil {
			t.Fatal(err)
		}
		if rec := call(artifact.ID); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "IMAGE_DECODE_FAILED") {
			t.Fatalf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	for name, test := range map[string]struct {
		data []byte
		code string
	}{
		"dimension": {pngConfigOnly(maxArtifactImageDimension+1, 1), "IMAGE_DIMENSION_EXCEEDED"},
		"pixels":    {pngConfigOnly(8_000, 5_001), "IMAGE_PIXEL_LIMIT_EXCEEDED"},
	} {
		rel := "artifacts/SES1/" + name + ".png"
		if err := os.WriteFile(filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel)), test.data, 0o644); err != nil {
			t.Fatal(err)
		}
		artifact, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: name, Kind: db.ArtifactImage, SourcePath: rel})
		if err != nil {
			t.Fatal(err)
		}
		if rec := call(artifact.ID); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), test.code) {
			t.Fatalf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	hugeRel := "artifacts/SES1/huge.png"
	hugePath := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(hugeRel))
	if err := os.WriteFile(hugePath, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(hugePath, maxArtifactImageSourceBytes+1); err != nil {
		t.Fatal(err)
	}
	huge, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "huge", Kind: db.ArtifactImage, SourcePath: hugeRel})
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(huge.ID); rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "IMAGE_TOO_LARGE_BYTES") {
		t.Fatalf("huge = %d %s", rec.Code, rec.Body.String())
	}
}
