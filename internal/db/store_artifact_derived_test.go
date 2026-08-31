package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDerivedArtifactPersistenceAndConcurrency(t *testing.T) {
	root := t.TempDir()
	database, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	original, err := database.CreateArtifact(context.Background(), Artifact{
		Title: "original", Kind: ArtifactImage, SourcePath: "artifacts/SES1/original.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := original
	results := make([]Artifact, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = database.CreateDerivedArtifact(context.Background(), Artifact{
				Title: "original — Düzenleme", SessionID: "SES1", Origin: "manual", DerivedFromArtifactID: original.ID,
			}, func(derived *Artifact, parent Artifact) (string, func() error, error) {
				if parent.ID != original.ID {
					return "", nil, fmt.Errorf("parent = %s", parent.ID)
				}
				return "artifacts/SES1/" + derived.ID + ".png", func() error { return nil }, nil
			})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].ID == results[1].ID || results[0].SourcePath == results[1].SourcePath {
		t.Fatalf("derived collision: %+v %+v", results[0], results[1])
	}
	for _, result := range results {
		if result.DerivedFromArtifactID != original.ID {
			t.Fatalf("parent not persisted: %+v", result)
		}
	}
	after, err := database.GetArtifact(context.Background(), original.ID)
	if err != nil || after != before {
		t.Fatalf("original changed: before=%+v after=%+v err=%v", before, after, err)
	}
	database.Close()
	reloaded, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	for _, result := range results {
		got, err := reloaded.GetArtifact(context.Background(), result.ID)
		if err != nil || got.DerivedFromArtifactID != original.ID {
			t.Fatalf("round trip = %+v, %v", got, err)
		}
	}
}

func TestDerivedArtifactWriteFailureLeavesNoMemoryOrFiles(t *testing.T) {
	root := t.TempDir()
	database, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	parent, err := database.CreateArtifact(context.Background(), Artifact{
		SessionID: "SES1", Title: "original", Kind: ArtifactImage,
		SourcePath: "artifacts/SES1/original.png",
	})
	if err != nil {
		t.Fatal(err)
	}

	staging := filepath.Join(t.TempDir(), "staging.png")
	target := filepath.Join(filepath.Dir(staging), "target.png")
	if err := os.WriteFile(staging, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	storeArtifacts := filepath.Join(root, dirArtifacts)
	if err := os.RemoveAll(storeArtifacts); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storeArtifacts, []byte("blocks artifact JSON directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	var derivedID string
	_, err = database.CreateDerivedArtifact(context.Background(), Artifact{
		SessionID: "SES1", Title: "derived", DerivedFromArtifactID: parent.ID,
	}, func(derived *Artifact, parent Artifact) (string, func() error, error) {
		derivedID = derived.ID
		if err := os.Rename(staging, target); err != nil {
			return "", nil, err
		}
		return "artifacts/SES1/" + derived.ID + ".png", func() error {
			return os.Remove(target)
		}, nil
	})
	if err == nil {
		t.Fatal("CreateDerivedArtifact succeeded despite blocked artifact JSON directory")
	}
	if _, getErr := database.GetArtifact(context.Background(), derivedID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("GetArtifact error = %v, want %v", getErr, ErrNotFound)
	}
	for _, path := range []string{staging, target} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("file remains at %s: %v", path, statErr)
		}
	}
}
