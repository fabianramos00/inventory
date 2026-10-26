package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookupRawTreatsBlankAsUnset(t *testing.T) {
	const key = "INVENTORY_TEST_LOOKUP_RAW"

	t.Setenv(key, "   ")
	if got, ok := lookupRaw(map[string]string{key: "from-file"}, key); !ok || got != "from-file" {
		t.Fatalf("blank env should fall back to file, got %q ok=%v", got, ok)
	}

	t.Setenv(key, "")
	if got, ok := lookupRaw(map[string]string{key: "  "}, key); ok {
		t.Fatalf("blank file value should be unset, got %q", got)
	}

	t.Setenv(key, "  from-env  ")
	if got, ok := lookupRaw(map[string]string{key: "from-file"}, key); !ok || got != "from-env" {
		t.Fatalf("env should win, got %q ok=%v", got, ok)
	}
}

func TestLoadRejectsInvalidSecrets(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"known placeholder", "replace-with-a-random-32-byte-secret"},
		{"other placeholder", "change-me"},
		{"short", "short-secret"},
		{"31 bytes", strings.Repeat("a", 31)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfig(t)
			t.Setenv("SECRET_KEY", tt.secret)

			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted secret %q", tt.secret)
			}
		})
	}
}

func TestLoadAccepts32ByteSecret(t *testing.T) {
	isolateConfig(t)
	secret := strings.Repeat("a", 32)
	t.Setenv("SECRET_KEY", secret)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretKey != secret {
		t.Fatalf("SecretKey = %q, want %q", cfg.SecretKey, secret)
	}
}

func TestLoadGeneratesSecretWhenEnvIsBlank(t *testing.T) {
	path := isolateConfig(t)
	t.Setenv("SECRET_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SecretKey) < 32 {
		t.Fatalf("generated secret is %d bytes, want at least 32", len(cfg.SecretKey))
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), "SECRET_KEY="+cfg.SecretKey) {
		t.Fatal("generated secret was not persisted to the config file")
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("config file mode = %04o, want 0600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("config dir mode = %04o, want 0700", got)
	}

	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if again.SecretKey != cfg.SecretKey {
		t.Fatal("generated secret was not reused on the next load")
	}
}

func TestLoadHardensConfigFilePermissions(t *testing.T) {
	path := isolateConfig(t)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("b", 32)
	if err := os.WriteFile(path, []byte("SECRET_KEY="+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET_KEY", "")

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("config file mode = %04o, want 0600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("config dir mode = %04o, want 0700", got)
	}
}

func TestLoadRejectsSymlinkConfigFile(t *testing.T) {
	path := isolateConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "config.env")
	secret := strings.Repeat("c", 32)
	if err := os.WriteFile(target, []byte("SECRET_KEY="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load followed a symlinked config file")
	}
}

func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	return filepath.Join(dir, "inventory-desktop", "config.env")
}
