package desktopruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipAliasesHardlinksAndPrivateFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "workspace ?#.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Shutdown(context.Background())
	for _, file := range []string{path, path + ".owner.lock"} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file protection: %v %v", info, err)
		}
	}
	alias := filepath.Join(directory, "alias.db")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if second, err := New(Config{SQLitePath: alias}); err == nil {
		_ = second.Shutdown(context.Background())
		t.Fatal("symlink bypassed owner")
	}
	hardlink := filepath.Join(directory, "hardlink.db")
	if err := os.Link(path, hardlink); err != nil {
		t.Fatal(err)
	}
	if second, err := New(Config{SQLitePath: hardlink}); err == nil {
		_ = second.Shutdown(context.Background())
		t.Fatal("hardlink bypassed owner")
	}
	// A filename containing URI query/fragment characters remains a literal path.
	var actualPath string
	if err := r.db.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&actualPath).Error; err != nil || actualPath != path {
		t.Fatalf("wrong database: %q %v", actualPath, err)
	}
}
