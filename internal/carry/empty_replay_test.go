package carry

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0then0/protocarry/internal/fixture"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestEmptyMessageSemanticsAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name, mode, field, policy string
		populated                 bool
		want                      Outcome
		code                      int
	}{
		{"default", "preserve", "routing_hint", "", false, InfrastructureError, 4},
		{"explicit-reject", "preserve", "routing_hint", EmptyOutputReject, false, InfrastructureError, 4},
		{"empty-preserve", "preserve", "routing_hint", EmptyOutputMessage, false, Pass, 0},
		{"only-new-field-copy", "copy", "routing_hint", EmptyOutputMessage, true, Fail, 1},
		{"optional-default-copy", "copy", "attempts", EmptyOutputMessage, true, Fail, 1},
		{"optional-absent-copy", "copy", "attempts", EmptyOutputMessage, false, Fail, 1},
		{"reject", "reject", "routing_hint", EmptyOutputMessage, false, Unresolved, 3},
		{"nonzero", "nonzero", "routing_hint", EmptyOutputMessage, false, InfrastructureError, 4},
		{"malformed", "malformed", "routing_hint", EmptyOutputMessage, false, InfrastructureError, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := setup(t, tc.mode)
			c.Fields, c.Controls, c.EmptyOutput = []string{tc.field}, nil, tc.policy
			m := dynamicpb.NewMessage(fixture.Descriptor(fixture.Demo(true), c.Message))
			if tc.populated {
				f := m.Descriptor().Fields().ByName(protoreflect.Name(tc.field))
				if tc.field == "attempts" {
					m.Set(f, protoreflect.ValueOfInt32(0))
				} else {
					m.Set(f, protoreflect.ValueOfString("future"))
				}
			}
			must(t, os.WriteFile(c.Seeds[0], fixture.MustMarshal(m), 0600))
			r, dir := check(t, c)
			outcome(t, r, tc.want)
			if ExitCode(r.Outcome) != tc.code || r.Executed != r.Planned {
				t.Fatalf("aggregate: %+v", r)
			}
			baseline := r.Cases[0]
			a := assertion(t, baseline, tc.field)
			switch tc.name {
			case "default", "explicit-reject":
				if baseline.Reason != "empty stdout: no transport evidence" || r.Cases[1].Outcome != Pass {
					t.Fatal(r.Cases)
				}
			case "empty-preserve":
				if r.Counts[Pass] != 2 || a.Actual == nil || a.Actual.Value.Value != "" || !*a.Preserved {
					t.Fatal(r.Cases)
				}
			case "only-new-field-copy":
				if a.Expected.Value.Value != "future" || a.Actual.Value.Value != "" || *a.Preserved || r.Counts[Fail] != 2 {
					t.Fatal(a)
				}
			case "optional-default-copy":
				if !*a.Expected.Value.Presence || *a.Actual.Value.Presence || *a.Preserved {
					t.Fatal(a)
				}
			case "optional-absent-copy":
				if baseline.Outcome != Pass || *a.Expected.Value.Presence || *a.Actual.Value.Presence || !*a.Preserved {
					t.Fatal(a)
				}
			default:
				if a.Actual != nil || a.Preserved != nil {
					t.Fatal("failure/refusal reached oracle", a)
				}
			}
			saved := readManifest(t, filepath.Join(dir, baseline.ID))
			wantPolicy := tc.policy
			if wantPolicy == "" {
				wantPolicy = EmptyOutputReject
			}
			if saved.Config.EmptyOutput != wantPolicy || saved.Format != 2 {
				t.Fatal(saved.Config)
			}
			replayDir := filepath.Join(t.TempDir(), "replay")
			rr, err := Replay(filepath.Join(dir, baseline.ID), replayDir)
			must(t, err)
			outcome(t, rr, baseline.Outcome)
			replaySaved := readManifest(t, filepath.Join(replayDir, "replay"))
			if replaySaved.Config.EmptyOutput != wantPolicy || replaySaved.ReplaySource == nil || replaySaved.ReplaySource.Tool != Version {
				t.Fatal(replaySaved)
			}
			if replaySaved.Case.Artifacts["input.bin"] != baseline.Artifacts["input.bin"] || !reflect.DeepEqual(replaySaved.Case.Assertions, baseline.Assertions) {
				t.Fatal("replay changed input/assertions")
			}
		})
	}
}

func readManifest(t *testing.T, dir string) Manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "case.json"))
	must(t, err)
	var m Manifest
	must(t, json.Unmarshal(b, &m))
	return m
}

func TestLegacyEvidenceReplay(t *testing.T) {
	// These are committed bundles produced by the actual v0.1.0 CLI, not edited
	// version strings on a new manifest. Overrides remove original checkout paths.
	for _, policy := range []string{"preserve", "empty"} {
		t.Run(policy, func(t *testing.T) {
			dir := filepath.Join("..", "..", "validation", "protobufjs", "evidence", "8.6.2-default-simple", "s001-baseline")
			original := readManifest(t, dir)
			if original.Engine.Tool != "0.1.0" || original.Format != 1 || original.Config.EmptyOutput != "" {
				t.Fatal("fixture is no longer legacy")
			}
			c := setup(t, policy)
			if policy == "preserve" {
				c.Adapter = []string{"cat"}
			}
			out := filepath.Join(t.TempDir(), "replay")
			r, err := ReplayWithOptions(dir, out, ReplayOptions{Adapter: c.Adapter, WorkingDir: c.WorkingDir})
			must(t, err)
			want := Pass
			if policy == "empty" {
				want = InfrastructureError
			}
			outcome(t, r, want)
			saved := readManifest(t, filepath.Join(out, "replay"))
			if saved.Config.EmptyOutput != EmptyOutputReject || saved.Engine.Tool != Version || saved.ReplaySource.Tool != "0.1.0" || r.ReplaySource.Tool != "0.1.0" || saved.Config.Runtime != (Runtime{}) {
				t.Fatal(saved)
			}
			if saved.Case.Artifacts["input.bin"] != original.Case.Artifacts["input.bin"] || r.Planned != 1 {
				t.Fatal("input hash/generation changed")
			}
			input, err := os.ReadFile(filepath.Join(dir, "input.bin"))
			must(t, err)
			replayed, err := os.ReadFile(filepath.Join(out, "replay", "input.bin"))
			must(t, err)
			if !bytes.Equal(input, replayed) {
				t.Fatal("input bytes changed")
			}
			for i, a := range saved.Case.Assertions {
				if !reflect.DeepEqual(a.Expected, original.Case.Assertions[i].Expected) || a.Path != original.Case.Assertions[i].Path || !reflect.DeepEqual(a.Numbers, original.Case.Assertions[i].Numbers) || a.Added != original.Case.Assertions[i].Added {
					t.Fatal("legacy assertions weakened")
				}
			}
			if policy == "empty" && r.Cases[0].Reason != "empty stdout: no transport evidence" {
				t.Fatal(r.Cases[0])
			}
		})
	}
}

func TestReplayRejectsBeforeAdapter(t *testing.T) {
	for _, change := range []string{"format", "future-producer", "format1-current", "format2-legacy", "missing-policy", "unknown-policy", "legacy-new-policy", "hash", "expectation", "assertions", "missing-descriptor"} {
		t.Run(change, func(t *testing.T) {
			c := setup(t, "preserve")
			_, dir := check(t, c)
			caseDir := filepath.Join(dir, "s001-baseline")
			m := readManifest(t, caseDir)
			switch change {
			case "format":
				m.Format = 99
			case "future-producer":
				m.Engine.Tool = "0.1.2"
			case "format1-current":
				m.Format = 1
			case "format2-legacy":
				m.Engine.Tool = "0.1.0"
			case "missing-policy":
				m.Config.EmptyOutput = ""
			case "unknown-policy":
				m.Config.EmptyOutput = "allow"
			case "legacy-new-policy":
				m.Format, m.Engine.Tool, m.Config.EmptyOutput = 1, "0.1.0", EmptyOutputMessage
			case "hash":
				must(t, os.WriteFile(filepath.Join(caseDir, "input.bin"), []byte{0}, 0600))
			case "expectation":
				m.Case.Assertions[0].Expected.Value.Value = "changed"
			case "assertions":
				m.Case.Assertions = m.Case.Assertions[1:]
			case "missing-descriptor":
				delete(m.Case.Artifacts, "old.pb")
			}
			must(t, writeJSON(filepath.Join(caseDir, "case.json"), m))
			marker := filepath.Join(t.TempDir(), "executed")
			out := filepath.Join(t.TempDir(), "out")
			_, err := ReplayWithOptions(caseDir, out, ReplayOptions{Adapter: []string{"sh", "-c", `touch "$1"`, "marker", marker}, WorkingDir: c.WorkingDir})
			var ce *ConfigError
			if !errors.As(err, &ce) {
				t.Fatal("invalid evidence accepted", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("adapter executed", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("invalid evidence created output", err)
			}
		})
	}
}

func TestUnknownEmptyOutputPreflight(t *testing.T) {
	c := setup(t, "preserve")
	c.EmptyOutput = "allow"
	path := filepath.Join(c.WorkingDir, "config.json")
	must(t, writeJSON(path, c))
	for _, policy := range []string{"allow", ""} {
		c.EmptyOutput = policy
		b, err := json.Marshal(c)
		must(t, err)
		var object map[string]any
		must(t, json.Unmarshal(b, &object))
		object["empty_output"] = policy
		must(t, writeJSON(path, object))
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "empty_output") {
			t.Fatal(err)
		}
	}
	c.EmptyOutput = "allow"
	_, err := Run(c, filepath.Join(t.TempDir(), "out"))
	var ce *ConfigError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "empty_output") {
		t.Fatal(err)
	}
}
