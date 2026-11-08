// External library compatibility case: kotlinx.serialization (Kotlin/JVM) through ProtoCarry.
//
// This is a measurement runner, not a unit test. It is skipped unless
// PROTOCARRY_KOTLIN_VALIDATION=1, because it needs the built adapter jar, a JDK and bin/protocarry.
// See README.md in this directory for the exact commands.
package kotlinserialization

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	wantKotlin        = "2.4.10"
	wantSerialization = "1.12.0"
)

type observation struct {
	ParentPresence []bool `json:"parent_presence"`
	Value          struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"value"`
}

type assertion struct {
	Path         string      `json:"path"`
	FieldNumbers []int       `json:"field_numbers"`
	Expected     observation `json:"expected"`
	Actual       observation `json:"actual"`
	Preserved    *bool       `json:"preserved"`
}

type caseReport struct {
	ID         string      `json:"id"`
	Outcome    string      `json:"outcome"`
	Assertions []assertion `json:"assertions"`
	Process    struct {
		ExitCode int  `json:"exit_code"`
		Started  bool `json:"started"`
		Timeout  bool `json:"timeout"`
	} `json:"process"`
}

type report struct {
	Engine struct {
		Tool string `json:"tool"`
	} `json:"engine"`
	Outcome       string         `json:"outcome"`
	Incomplete    bool           `json:"incomplete"`
	Counts        map[string]int `json:"counts"`
	PlannedCases  int            `json:"planned_cases"`
	AttemptedCase int            `json:"attempted_cases"`
	ExecutedCases int            `json:"executed_cases"`
	Cases         []caseReport   `json:"cases"`
}

type mode struct {
	name        string
	outcome     string
	exitCode    int
	lossAtRoot  bool // the added root field is lost
	lossInChild bool // the added field inside the known child message is lost
}

var modes = []mode{
	{"no-holder", "FAIL", 1, true, true},
	{"preserve-holder", "PASS", 0, false, false},
	{"rebuild", "FAIL", 1, true, true},
}

var controlPaths = []string{"child.label", "id", "revision"}
var addedPaths = []string{"child.future_note", "future_note"}

type runtimeInfo struct {
	JDK           string `json:"jdk"`
	JVMVendor     string `json:"jvm_vendor"`
	KotlinRuntime string `json:"kotlin_runtime"`
	KotlinCompile string `json:"kotlin_compiler"`
	Serialization string `json:"kotlinx_serialization_protobuf"`
}

func TestKotlinSerializationCompatibility(t *testing.T) {
	if os.Getenv("PROTOCARRY_KOTLIN_VALIDATION") != "1" {
		t.Skip("set PROTOCARRY_KOTLIN_VALIDATION=1 after building the adapter jar; see README.md")
	}
	dir, err := os.Getwd()
	must(t, err)
	bin := os.Getenv("PROTOCARRY_BIN")
	if bin == "" {
		bin = filepath.Join(dir, "..", "..", "bin", "protocarry")
	}
	bin, err = filepath.Abs(bin)
	must(t, err)
	if _, err := os.Stat(filepath.Join(dir, "build", "libs", "adapter.jar")); err != nil {
		t.Fatalf("adapter jar is missing; build it with ./gradlew jar: %v", err)
	}
	runs := filepath.Join(dir, "runs")
	if _, err := os.Stat(runs); err == nil {
		t.Fatalf("%s exists; move previous measurements aside before running again", runs)
	}
	must(t, os.MkdirAll(runs, 0o755))
	// ProtoCarry and the adapter must not depend on the current working directory.
	elsewhere := t.TempDir()

	summary := map[string]any{}
	var seen runtimeInfo
	results := map[string]any{}
	for _, m := range modes {
		out := filepath.Join(runs, m.name)
		exit, stdout := protocarry(t, bin, elsewhere, "check", "-config", filepath.Join(dir, m.name+".json"), "-out", out)
		if exit != m.exitCode {
			t.Fatalf("%s: exit %d, want %d\n%s", m.name, exit, m.exitCode, stdout)
		}
		rep := readReport(t, filepath.Join(out, "report.json"))
		checkReport(t, m, rep)
		seen = checkEvidence(t, m, out, rep)
		results[m.name] = map[string]any{"outcome": rep.Outcome, "exit_code": exit, "counts": rep.Counts, "failed_paths": failedPaths(rep)}
		t.Logf("%s: %s %v failed_paths=%v", m.name, rep.Outcome, rep.Counts, failedPaths(rep))
	}
	if seen.KotlinRuntime != wantKotlin || seen.KotlinCompile != wantKotlin || seen.Serialization != wantSerialization {
		t.Fatalf("unexpected versions: %+v", seen)
	}
	summary["runtime"] = seen
	summary["modes"] = results

	// Replay every saved case from this run. Every replay must reproduce the case's outcome.
	var replays []string
	for _, m := range modes {
		cases := caseDirs(t, filepath.Join(runs, m.name))
		for _, c := range cases {
			replayCase(t, bin, elsewhere, m, c, filepath.Join(runs, "replay-fresh-"+m.name+"-"+filepath.Base(c)), nil)
			replays = append(replays, m.name+"/"+filepath.Base(c))
		}
	}

	// Replay the committed evidence from another working directory, with portable overrides.
	// Skipped only when the evidence has not been recorded yet.
	committed := filepath.Join(dir, "evidence")
	if _, err := os.Stat(committed); err == nil {
		for _, m := range modes {
			for _, c := range caseDirs(t, filepath.Join(committed, m.name)) {
				override := []string{"-working-dir", dir, "-adapter", fmt.Sprintf(`["java","-jar","build/libs/adapter.jar","%s"]`, m.name)}
				replayCase(t, bin, elsewhere, m, c, filepath.Join(runs, "replay-committed-"+m.name+"-"+filepath.Base(c)), override)
				replays = append(replays, "committed:"+m.name+"/"+filepath.Base(c))
			}
		}
	}
	summary["replayed_cases"] = replays

	// A FAIL must mean semantic loss. Broken launches must be infrastructure errors instead.
	summary["infrastructure_controls"] = infrastructureControls(t, bin, elsewhere, dir, runs)

	// The Kotlin-side re-decode check runs in Gradle (AdapterTest); record that it is a separate check.
	summary["kotlin_redecode_check"] = "./gradlew test (AdapterTest)"

	data, err := json.MarshalIndent(summary, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(runs, "summary.json"), append(data, '\n'), 0o644))

	if os.Getenv("PROTOCARRY_KOTLIN_RECORD") == "1" {
		record(t, dir, runs, data)
	}
}

func checkReport(t *testing.T, m mode, rep report) {
	t.Helper()
	if rep.Engine.Tool != "0.1.1" || rep.Incomplete || rep.Outcome != m.outcome {
		t.Fatalf("%s: tool=%s incomplete=%v outcome=%s", m.name, rep.Engine.Tool, rep.Incomplete, rep.Outcome)
	}
	if rep.PlannedCases != 3 || rep.AttemptedCase != 3 || rep.ExecutedCases != 3 {
		t.Fatalf("%s: planned/attempted/executed = %d/%d/%d, want 3/3/3", m.name, rep.PlannedCases, rep.AttemptedCase, rep.ExecutedCases)
	}
	if rep.Counts["INFRASTRUCTURE_ERROR"] != 0 || rep.Counts["UNRESOLVED"] != 0 || rep.Counts[m.outcome] != 3 {
		t.Fatalf("%s: counts %v", m.name, rep.Counts)
	}
	var ids []string
	for _, c := range rep.Cases {
		ids = append(ids, c.ID)
		if c.Outcome != m.outcome || !c.Process.Started || c.Process.Timeout || c.Process.ExitCode != 0 {
			t.Fatalf("%s/%s: outcome %s process %+v: a FAIL must come from a clean adapter run", m.name, c.ID, c.Outcome, c.Process)
		}
		checkAssertions(t, m, c)
	}
	if want := []string{"s001-baseline", "s001-f001-value", "s001-f002-value"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("%s: cases %v, want %v", m.name, ids, want)
	}
}

func checkAssertions(t *testing.T, m mode, c caseReport) {
	t.Helper()
	got := map[string]assertion{}
	for _, a := range c.Assertions {
		got[a.Path] = a
		if a.Preserved == nil {
			t.Fatalf("%s/%s: %s was not evaluated", m.name, c.ID, a.Path)
		}
	}
	var paths []string
	for p := range got {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if want := []string{"child.future_note", "child.label", "future_note", "id", "revision"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("%s/%s: assertion paths %v, want %v", m.name, c.ID, paths, want)
	}
	// Controls are known to the old schema and must survive in every mode, even when added fields are lost.
	for _, p := range controlPaths {
		a := got[p]
		if !*a.Preserved || !reflect.DeepEqual(a.Expected, a.Actual) || a.Expected.Value.Value == "" || a.Expected.Value.Value == "0" {
			t.Fatalf("%s/%s: control %s not preserved with a nondefault value: %+v", m.name, c.ID, p, a)
		}
	}
	numbers := map[string][]int{"future_note": {3}, "child.future_note": {4, 2}}
	lost := map[string]bool{"future_note": m.lossAtRoot, "child.future_note": m.lossInChild}
	for _, p := range addedPaths {
		a := got[p]
		if !reflect.DeepEqual(a.FieldNumbers, numbers[p]) || a.Expected.Value.Kind != "string" || a.Expected.Value.Value == "" {
			t.Fatalf("%s/%s: %s numbers %v expected %+v", m.name, c.ID, p, a.FieldNumbers, a.Expected)
		}
		if lost[p] {
			// Exact loss: the field decodes as the semantic default while its parent is still present.
			if *a.Preserved || a.Actual.Value.Value != "" || !reflect.DeepEqual(a.Actual.ParentPresence, a.Expected.ParentPresence) {
				t.Fatalf("%s/%s: %s should be lost with an empty actual value: %+v", m.name, c.ID, p, a)
			}
		} else if !*a.Preserved || !reflect.DeepEqual(a.Actual, a.Expected) {
			t.Fatalf("%s/%s: %s should be preserved: %+v", m.name, c.ID, p, a)
		}
	}
	if c.ID == "s001-baseline" {
		if got["future_note"].Expected.Value.Value != "future-root" || got["child.future_note"].Expected.Value.Value != "future-child" {
			t.Fatalf("%s: baseline expectations differ from the seed", m.name)
		}
	}
}

// checkEvidence validates the adapter's own stderr diagnostics for the baseline case and returns the runtime versions.
func checkEvidence(t *testing.T, m mode, out string, rep report) runtimeInfo {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "s001-baseline", "stderr.log"))
	must(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var head struct {
		Mode       string      `json:"mode"`
		InputBytes int         `json:"input_bytes"`
		Runtime    runtimeInfo `json:"runtime"`
	}
	must(t, json.Unmarshal([]byte(lines[0]), &head))
	if head.Mode != m.name || head.InputBytes != 40 {
		t.Fatalf("%s: stderr header %+v", m.name, head)
	}
	diagnostics := map[string]string{
		"no-holder":       "",
		"preserve-holder": `{"unknown_bytes":{"root":13,"child":14}}`,
		"rebuild":         `{"unknown_bytes_decoded":{"root":13,"child":14},"unknown_bytes_rebuilt":{"root":0,"child":0}}`,
	}[m.name]
	outputBytes := map[string]int{"no-holder": 13, "preserve-holder": 40, "rebuild": 13}[m.name]
	want := []string{lines[0], fmt.Sprintf(`{"output_bytes":%d}`, outputBytes)}
	if diagnostics != "" {
		want = []string{lines[0], diagnostics, want[1]}
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("%s: stderr\n%s\nwant\n%s", m.name, strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	return head.Runtime
}

func failedPaths(rep report) map[string][]string {
	failed := map[string][]string{}
	for _, c := range rep.Cases {
		for _, a := range c.Assertions {
			if a.Preserved != nil && !*a.Preserved {
				failed[c.ID] = append(failed[c.ID], fmt.Sprintf("%s expected %q actual %q", a.Path, a.Expected.Value.Value, a.Actual.Value.Value))
			}
		}
		sort.Strings(failed[c.ID])
	}
	return failed
}

func replayCase(t *testing.T, bin, cwd string, m mode, caseDir, out string, extra []string) {
	t.Helper()
	args := append([]string{"replay", "-case", caseDir, "-out", out}, extra...)
	exit, stdout := protocarry(t, bin, cwd, args...)
	if exit != m.exitCode {
		t.Fatalf("replay %s: exit %d, want %d\n%s", caseDir, exit, m.exitCode, stdout)
	}
	data, err := os.ReadFile(filepath.Join(out, "report.json"))
	must(t, err)
	var rep report
	must(t, json.Unmarshal(data, &rep))
	if rep.Outcome != m.outcome || len(rep.Cases) != 1 || rep.Cases[0].Outcome != m.outcome {
		t.Fatalf("replay %s: outcome %s", caseDir, rep.Outcome)
	}
	checkAssertions(t, m, rep.Cases[0])
}

func infrastructureControls(t *testing.T, bin, cwd, dir, runs string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "preserve-holder.json"))
	must(t, err)
	controls := map[string]any{}
	for name, adapter := range map[string][]string{
		"missing-jar":  {"java", "-jar", "build/libs/does-not-exist.jar", "preserve-holder"},
		"unknown-mode": {"java", "-jar", "build/libs/adapter.jar", "bogus"},
	} {
		var cfg map[string]any
		must(t, json.Unmarshal(data, &cfg))
		for _, key := range []string{"old_descriptor", "new_descriptor"} {
			cfg[key] = filepath.Join(dir, cfg[key].(string))
		}
		cfg["seeds"] = []string{filepath.Join(dir, "seed.bin")}
		cfg["adapter"] = adapter
		cfg["working_dir"] = dir
		cfgData, err := json.Marshal(cfg)
		must(t, err)
		path := filepath.Join(runs, "control-"+name+".json")
		must(t, os.WriteFile(path, cfgData, 0o644))
		out := filepath.Join(runs, "control-"+name)
		exit, stdout := protocarry(t, bin, cwd, "check", "-config", path, "-out", out)
		rep := readReport(t, filepath.Join(out, "report.json"))
		// JVM start failures and adapter errors are infrastructure errors (exit 4), never a semantic FAIL.
		if exit != 4 || rep.Outcome != "INFRASTRUCTURE_ERROR" || rep.Counts["FAIL"] != 0 || rep.Counts["PASS"] != 0 {
			t.Fatalf("control %s: exit %d report %+v\n%s", name, exit, rep.Counts, stdout)
		}
		controls[name] = map[string]any{"outcome": rep.Outcome, "exit_code": exit, "counts": rep.Counts}
	}
	return controls
}

func record(t *testing.T, dir, runs string, summary []byte) {
	t.Helper()
	committed := filepath.Join(dir, "evidence")
	if _, err := os.Stat(committed); err == nil {
		t.Fatalf("%s exists; move it aside before recording", committed)
	}
	for _, m := range modes {
		must(t, copyTree(filepath.Join(runs, m.name), filepath.Join(committed, m.name)))
	}
	must(t, os.WriteFile(filepath.Join(dir, "results.json"), summary, 0o644))
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func caseDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	must(t, err)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	if len(dirs) != 3 {
		t.Fatalf("%s: %d case directories, want 3", root, len(dirs))
	}
	return dirs
}

func readReport(t *testing.T, path string) report {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	var rep report
	must(t, json.Unmarshal(data, &rep))
	return rep
}

func protocarry(t *testing.T, bin, cwd string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, buf.String()
	case errors.As(err, &exit):
		return exit.ExitCode(), buf.String()
	default:
		t.Fatal(err)
		return -1, ""
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
