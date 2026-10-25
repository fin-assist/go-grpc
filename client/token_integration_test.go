package client

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestIntegrationProjectedTokenAtomicRotationAndConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := &FileToken{Path: path}
	for _, want := range []string{"one", "rotated-token"} {
		if want != "one" {
			next := filepath.Join(dir, "next")
			if err := os.WriteFile(next, []byte(want), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(next, path); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				token, err := source.Token(t.Context())
				if err != nil || token != want {
					t.Errorf("rotation result %q %v", token, err)
				}
			})
		}
		wg.Wait()
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(t.Context()); err == nil {
		t.Fatal("missing file reused stale credentials")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(t.Context()); err == nil {
		t.Fatal("empty file reused stale credentials")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(t.Context()); err == nil {
		t.Fatal("directory accepted as credential")
	}
	if _, err := DevToken("").Token(t.Context()); err == nil {
		t.Fatal("empty dev credential accepted")
	}
}
