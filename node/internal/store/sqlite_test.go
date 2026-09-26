package store

import (
	"path/filepath"
	"testing"
)

// A database written in WAL mode opens in shared mode with its data intact
// and switches to the rollback journal; a second opener does not fail.
func TestSharedOpenSwitchesFromWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	a, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	a.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Fatalf("local open is %s, want wal", mode)
	}
	// Another process still has it open: the shared open must not fail.
	b, err := OpenSQLiteShared(path)
	if err != nil {
		t.Fatalf("shared open failed while another had it open: %v", err)
	}
	b.Close()
	a.Close()
	c, err := OpenSQLiteShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "delete" {
		t.Fatalf("shared open left it in %s", mode)
	}
}
