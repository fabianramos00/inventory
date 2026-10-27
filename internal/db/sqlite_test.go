package db

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenSQLiteTightensExistingParentDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}

	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	conn, err := OpenSQLite(filepath.Join(dir, "inventory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("existing directory mode = %04o, want 0700", got)
	}
}

func TestOpenSQLiteRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.sqlite")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	conn, err := OpenSQLite(link)
	if err == nil {
		conn.Close()
		t.Fatal("OpenSQLite accepted a symlink database path")
	}
}

func TestOpenSQLiteFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}

	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	conn, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	// Force WAL activity so sidecars are created when the driver supports it.
	if _, err := conn.Exec("CREATE TABLE permission_probe (id INTEGER)"); err != nil {
		t.Fatal(err)
	}

	mainInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := mainInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database file mode = %04o, want 0600", got)
	}

	for _, candidate := range []string{path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %04o, want 0600", filepath.Base(candidate), got)
		}
	}
}
