package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadJWTExpire(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
app:
  mode: dev
  addr: ":8080"
jwt:
  secret: test
  expire: 24h
transcode:
  poll_interval: 5s
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWT.Expire != 24*time.Hour {
		t.Fatalf("expire = %s", cfg.JWT.Expire)
	}
	if cfg.Transcode.PollInterval != 5*time.Second {
		t.Fatalf("poll = %s", cfg.Transcode.PollInterval)
	}
}
