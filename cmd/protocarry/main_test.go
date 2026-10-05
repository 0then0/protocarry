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

func TestCLIEmptyOutputOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, policy, adapter string
		seed                  []byte
		code                  int
		text                  string
	}{
		{"default", "reject", "cat", nil, 4, "empty stdout: no transport evidence"},
		{"preserve", "message", "cat", nil, 0, "PASS: planned=2"},
		{"discard", "message", "discard", []byte{0x12, 6, 'f', 'u', 't', 'u', 'r', 'e'}, 1, `expected: "future" (string)`},
		{"refusal", "message", "reject", nil, 3, "UNRESOLVED"},
		{"nonzero", "message", "nonzero", nil, 4, "INFRASTRUCTURE_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(os.WriteFile(filepath.Join(dir, "old.pb"), fixture.MustMarshal(fixture.External(false)), 0600))
			must(os.WriteFile(filepath.Join(dir, "new.pb"), fixture.MustMarshal(fixture.External(true)), 0600))
			must(os.WriteFile(filepath.Join(dir, "seed.bin"), tc.seed, 0600))
			argv := []string{"cat"}
			switch tc.adapter {
			case "discard":
				argv = []string{"sh", "-c", "cat >/dev/null"}
			case "reject":
				argv = []string{"sh", "-c", "exit 75"}
			case "nonzero":
				argv = []string{"sh", "-c", "exit 42"}
			}
			c := carry.Config{OldDescriptor: "old.pb", NewDescriptor: "new.pb", Message: "validation.Envelope", Seeds: []string{"seed.bin"}, Adapter: argv, Fields: []string{"future_note"}, EmptyOutput: tc.policy, Limits: carry.DefaultLimits()}
			data, err := json.Marshal(c)
			must(err)
			config := filepath.Join(dir, "config.json")
			must(os.WriteFile(config, data, 0600))
			outDir := filepath.Join(dir, "evidence")
			var out, logs bytes.Buffer
			if code := cli([]string{"check", "-config", config, "-out", outDir}, &out, &logs); code != tc.code || !strings.Contains(out.String(), tc.text) {
				t.Fatal(code, out.String(), logs.String())
			}
			if tc.name == "discard" && !strings.Contains(out.String(), `actual:   "" (string)`) {
				t.Fatal(out.String())
			}
			if _, err := os.Stat(filepath.Join(outDir, "report.json")); err != nil {
				t.Fatal("outcome without evidence", err)
			}
		})
	}
}

func TestCLIRejectsInvalidOutputPolicyBeforeAdapter(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"old.pb":   fixture.MustMarshal(fixture.External(false)),
		"new.pb":   fixture.MustMarshal(fixture.External(true)),
		"seed.bin": {},
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, policy := range []any{nil, "", "allow", true, 1, []any{}, map[string]any{}} {
		t.Run(stringifyPolicy(policy), func(t *testing.T) {
			work := t.TempDir()
			marker := filepath.Join(work, "executed")
			c := map[string]any{
				"old_descriptor": filepath.Join(dir, "old.pb"), "new_descriptor": filepath.Join(dir, "new.pb"),
				"message": "validation.Envelope", "seeds": []string{filepath.Join(dir, "seed.bin")},
				"fields": []string{"future_note"}, "empty_output": policy,
				"adapter": []string{"sh", "-c", `touch "$1"; cat`, "marker", marker},
			}
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(work, "config.json")
			if err := os.WriteFile(config, data, 0600); err != nil {
				t.Fatal(err)
			}
			evidence := filepath.Join(work, "evidence")
			var out, logs bytes.Buffer
			if code := cli([]string{"check", "-config", config, "-out", evidence}, &out, &logs); code != 2 || out.Len() != 0 || !strings.Contains(logs.String(), "empty_output") {
				t.Fatal(code, out.String(), logs.String())
			}
			for _, path := range []string{marker, evidence} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("invalid policy executed adapter or created evidence", path, err)
				}
			}
		})
	}
}

func stringifyPolicy(policy any) string {
	data, _ := json.Marshal(policy)
	return string(data)
}
