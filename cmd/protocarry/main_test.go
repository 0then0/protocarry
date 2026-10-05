package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0then0/protocarry/internal/carry"
	"github.com/0then0/protocarry/internal/fixture"
)

func TestCLIAndPortableReplay(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "old.pb"), fixture.MustMarshal(fixture.Demo(false)), 0600))
	must(os.WriteFile(filepath.Join(dir, "new.pb"), fixture.MustMarshal(fixture.Demo(true)), 0600))
	must(os.WriteFile(filepath.Join(dir, "seed.bin"), fixture.DemoSeed(), 0600))
	c := carry.Config{OldDescriptor: "old.pb", NewDescriptor: "new.pb", Message: "demo.Envelope", Seeds: []string{"seed.bin"}, Adapter: []string{"cat"}, Fields: []string{"routing_hint"}, Controls: []string{"id"}, Limits: carry.DefaultLimits()}
	data, err := json.Marshal(c)
	must(err)
	config := filepath.Join(dir, "config.json")
	must(os.WriteFile(config, data, 0600))
	outDir := filepath.Join(dir, "evidence")
	var output, logs bytes.Buffer
	if code := cli([]string{"check", "-config", config, "-out", outDir}, &output, &logs); code != 0 {
		t.Fatal(code, logs.String(), output.String())
	}
	if !strings.Contains(output.String(), "PASS: planned=2") {
		t.Fatal(output.String())
	}
	if _, err = os.Stat(filepath.Join(outDir, "report.json")); err != nil {
		t.Fatal("outcome without evidence", err)
	}
	replayOut := filepath.Join(dir, "replay")
	output.Reset()
	logs.Reset()
	if code := cli([]string{"replay", "-case", filepath.Join(outDir, "s001-baseline"), "-out", replayOut, "-working-dir", dir, "-adapter", `["cat"]`}, &output, &logs); code != 0 {
		t.Fatal(code, logs.String())
	}
	original, err := os.ReadFile(filepath.Join(outDir, "s001-baseline/input.bin"))
	must(err)
	replayed, err := os.ReadFile(filepath.Join(replayOut, "replay/input.bin"))
	must(err)
	if !bytes.Equal(original, replayed) {
		t.Fatal("replay changed input")
	}
	output.Reset()
	logs.Reset()
	if code := cli([]string{"check", "-config", config, "-out", outDir}, &output, &logs); code != 4 || strings.Contains(output.String(), "PASS") {
		t.Fatal("existing evidence overwritten or false PASS", code)
	}
}
func TestConfigStrictAndCLIUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}, {"check"}, {"replay", "-case", "x", "-out", "y", "-adapter", "null"}} {
		var out, logs bytes.Buffer
		if code := cli(args, &out, &logs); code != 2 {
			t.Fatal(args, code)
		}
	}
	var out, logs bytes.Buffer
	if code := cli([]string{"version"}, &out, &logs); code != 0 || !strings.Contains(out.String(), carry.Version) {
		t.Fatal(code, out.String())
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	if err := os.WriteFile(path, []byte(`{"typo":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := carry.LoadConfig(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatal(err)
	}
}
