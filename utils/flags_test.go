package utils

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v2"
)

type sampleOptions struct {
	Host    string `hcl:"host" flagName:"host" flagSName:"H" flagDescribe:"Host address" default:"127.0.0.1"`
	Port    int    `hcl:"port" flagName:"port" flagSName:"p" flagDescribe:"Port number" default:"8080"`
	Enabled bool   `hcl:"enabled" flagName:"enabled" flagDescribe:"Enable feature" default:"true"`
	Disabled bool  `hcl:"disabled" flagName:"disabled" flagDescribe:"Disable feature" default:"false"`
	NoTag   string
}

func TestApplyDefaultValues_AndFlags_AndConfigFile(t *testing.T) {
	opts := &sampleOptions{}
	if err := ApplyDefaultValues(opts); err != nil {
		t.Fatalf("ApplyDefaultValues failed: %v", err)
	}
	if opts.Host != "127.0.0.1" || opts.Port != 8080 || !opts.Enabled || opts.Disabled {
		t.Fatalf("unexpected defaults: %+v", opts)
	}

	flags, mappings, err := GenerateFlags(opts)
	if err != nil {
		t.Fatalf("GenerateFlags failed: %v", err)
	}
	if len(flags) != 4 || mappings["host"] != "Host" {
		t.Fatalf("unexpected flags/mappings: len=%d mappings=%v", len(flags), mappings)
	}

	// Test ApplyConfigFile
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "gotty.hcl")
	if err := os.WriteFile(cfgPath, []byte(`host = "0.0.0.0"`+"\n"+`port = 9090`+"\n"+`enabled = false`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ApplyConfigFile(cfgPath, opts); err != nil {
		t.Fatalf("ApplyConfigFile failed: %v", err)
	}
	if opts.Host != "0.0.0.0" || opts.Port != 9090 || opts.Enabled {
		t.Fatalf("unexpected config values after HCL load: %+v", opts)
	}

	// Test ApplyFlags via urfave/cli Context
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range flags {
		_ = f.Apply(set)
	}
	_ = set.Parse([]string{"--host", "localhost", "--port", "3000", "--enabled"})
	ctx := cli.NewContext(cli.NewApp(), set, nil)
	ApplyFlags(flags, mappings, ctx, opts)

	if opts.Host != "localhost" || opts.Port != 3000 || !opts.Enabled {
		t.Fatalf("unexpected values after ApplyFlags: %+v", opts)
	}
}
