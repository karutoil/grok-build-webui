package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchDirs(t *testing.T) {
	root := t.TempDir()
	mkdir := func(rel string) {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("work/api-server")
	mkdir("work/web-app")
	mkdir("work/node_modules/dep")          // skipped: dependency dir
	mkdir("other/.hidden-project")          // skipped: hidden dir
	mkdir("Deep/Nested/Folder/myproj")      // beyond max depth from root? no, depth 4 ok

	got := searchDirs(root, "", "proj")
	var names []string
	for _, m := range got {
		names = append(names, filepath.Base(m.Path))
	}
	if len(got) != 1 || names[0] != "myproj" {
		t.Fatalf("expected only [myproj], got %v", names)
	}

	// Case-insensitive substring match.
	if res := searchDirs(root, "", "SERVER"); len(res) != 1 || res[0].Name != "api-server" {
		t.Fatalf("expected api-server match, got %+v", res)
	}

	// No query in node_modules children, even on direct match request.
	if res := searchDirs(root, "", "dep"); len(res) != 0 {
		t.Fatalf("node_modules should be skipped, got %+v", res)
	}
}

func TestSearchDirsDepthCapAndSorted(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a/b/c/d/e/f/toolong")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if res := searchDirs(root, "", "toolong"); len(res) != 0 {
		t.Fatalf("depth cap should hide toolong, got %+v", res)
	}

	if err := os.MkdirAll(filepath.Join(root, "zeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	all := searchDirs(root, "", "")
	if len(all) < 2 {
		return // nothing to sort-check
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Name > all[i].Name {
			t.Fatalf("results not sorted by name: %+v", all)
		}
	}
}
