package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

const baseConfig = `
clickhouse:
  host: localhost
  port: 9000
  database: nodelistdb
`

func TestNetworksDefaultInjected(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.Networks) != 1 || cfg.Networks[0].Name != "fidonet" {
		t.Fatalf("expected injected fidonet default, got %+v", cfg.Networks)
	}

	pattern := cfg.Network("fidonet").Pattern()
	for _, name := range []string{"nodelist.216", "nodelist_2024_001", "nodelist"} {
		if !pattern.MatchString(name) {
			t.Errorf("fidonet pattern must match %q", name)
		}
	}
	if pattern.MatchString("fsxnet.191") {
		t.Error("fidonet pattern must not match fsxnet.191")
	}
}

func TestNetworksCustomEntries(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, baseConfig+`
networks:
  - name: fidonet
  - name: fsxnet
`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	fsx := cfg.Network("fsxnet")
	if fsx == nil {
		t.Fatal("fsxnet network not found")
	}
	if !fsx.Pattern().MatchString("fsxnet.191") {
		t.Error("default fsxnet pattern must match fsxnet.191")
	}
	if fsx.Pattern().MatchString("nodelist.191") {
		t.Error("fsxnet pattern must not match nodelist.191")
	}
	if cfg.Network("unknown") != nil {
		t.Error("unknown network must return nil")
	}
}

func TestNetworksValidation(t *testing.T) {
	cases := map[string]string{
		"uppercase name": baseConfig + `
networks:
  - name: FSXNET
`,
		"duplicate name": baseConfig + `
networks:
  - name: fsxnet
  - name: fsxnet
`,
		"invalid regex": baseConfig + `
networks:
  - name: fsxnet
    nodelist_pattern: "["
`,
	}

	for label, content := range cases {
		if _, err := LoadConfig(writeConfig(t, content)); err == nil {
			t.Errorf("%s: expected validation error", label)
		}
	}
}
