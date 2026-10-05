package carry

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"google.golang.org/protobuf/types/dynamicpb"
)

type ConfigError struct{ Err error }

func (e *ConfigError) Error() string { return "configuration: " + e.Err.Error() }
func configError(err error) error    { return &ConfigError{err} }

type prepared struct {
	old, new    Schema
	paths       []Path
	seeds       []*dynamicpb.Message
	raw         [][]byte
	unsupported string
}

// All config/schema/seed resources are preflighted before any adapter invocation.
func prepare(c Config) (prepared, error) {
	p := prepared{}
	if err := c.Validate(); err != nil {
		return p, configError(err)
	}
	var err error
	p.old, err = ReadSchema(c.OldDescriptor, c.Message)
	if err != nil {
		return p, configError(err)
	}
	p.new, err = ReadSchema(c.NewDescriptor, c.Message)
	if err != nil {
		return p, configError(err)
	}
	if err = CheckEvolution(p.old.Root, p.new.Root); err != nil {
		p.unsupported = err.Error()
	}
	for i, text := range append(append([]string{}, c.Fields...), c.Controls...) {
		path, e := ResolvePath(p.old.Root, p.new.Root, text, c.Limits.MaxDepth)
		if e != nil {
			return p, configError(e)
		}
		if p.unsupported == "" {
			if i < len(c.Fields) && !path.Added {
				return p, configError(fmt.Errorf("fields path %q is not an addition; use controls for known fields", text))
			}
			if i >= len(c.Fields) && path.Added {
				return p, configError(fmt.Errorf("control %q is not known to the old schema", text))
			}
		}
		p.paths = append(p.paths, path)
		if path.Unsupported != "" && p.unsupported == "" {
			p.unsupported = text + ": " + path.Unsupported
		}
	}
	totalSeedBytes := 0
	for _, file := range c.Seeds {
		b, e := readBounded(file, c.Limits.MaxMessageBytes)
		if e != nil {
			return p, configError(e)
		}
		totalSeedBytes += len(b)
		if totalSeedBytes > 16<<20 {
			return p, configError(fmt.Errorf("total seed bytes exceed 16 MiB"))
		}
		m, e := decodeMessage(p.new, b)
		if e != nil {
			return p, configError(fmt.Errorf("invalid seed %s: %w", file, e))
		}
		if e = checkValueSizes(m, c.Limits.MaxValueBytes); e != nil {
			return p, configError(fmt.Errorf("seed %s: %w", file, e))
		}
		p.raw = append(p.raw, b)
		p.seeds = append(p.seeds, m)
	}
	return p, nil
}

func Run(c Config, out string) (*Report, error) {
	if c.EmptyOutput == "" {
		c.EmptyOutput = EmptyOutputReject
	}
	p, err := prepare(c)
	if err != nil {
		return nil, err
	}
	plans := makePlans(c, p.seeds, p.raw, p.paths)
	if err = newEvidence(out); err != nil {
		return nil, err
	}
	r := &Report{Format: 2, Engine: engine(), Runtime: c.Runtime, Message: c.Message, Fields: p.paths, Cases: []Case{}}
	attempts := 0
	for _, pl := range plans {
		reason := pl.Reason
		if p.unsupported != "" {
			reason = "unsupported model: " + p.unsupported
		}
		if reason == "" && attempts >= c.Limits.MaxCases {
			reason = "max_cases reached: planned case not executed"
		}
		if reason == "" && pl.Kind != "baseline" {
			for _, path := range p.paths {
				if path.Text == pl.Field {
					pl = generate(c, p.new, pl, path)
					break
				}
			}
			reason = pl.Reason
		}
		item := execute(c, p, pl, reason)
		if item.Attempted {
			attempts++
		}
		if err = persist(filepath.Join(out, pl.ID), c, p.old, p.new, p.paths, &item, pl.Input, nil); err != nil {
			return nil, err
		}
		item.Process.Stdout = nil
		item.Process.Stderr = nil
		r.Cases = append(r.Cases, item)
	}
	aggregate(r)
	if err = writeJSON(filepath.Join(out, "report.json"), r); err != nil {
		return nil, err
	}
	return r, nil
}

func execute(c Config, p prepared, pl plan, skip string) Case {
	item := Case{ID: pl.ID, Seed: pl.Seed, Kind: pl.Kind, Field: pl.Field, Evidence: pl.ID, Assertions: []Assertion{}, Process: Process{ExitCode: -1}}
	if pl.Message != nil && p.unsupported == "" {
		item.Assertions = assertInput(pl.Message, p.paths)
	}
	if skip != "" {
		item.Outcome = Unresolved
		item.Reason = skip
		return item
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		item.Outcome = InfrastructureError
		item.Reason = "v0.1 supports macOS and Linux subprocesses only"
		return item
	}
	item.Attempted = true
	item.Process = runProcess(c, pl.Input)
	item.Executed = item.Process.Started
	if item.Process.Error != "" {
		item.Outcome = InfrastructureError
		item.Reason = item.Process.Error
		return item
	}
	if item.Process.ExitCode == RejectExit {
		item.Outcome = Unresolved
		item.Reason = "adapter controlled rejection (exit 75)"
		return item
	}
	output, err := decodeMessage(p.new, item.Process.Stdout)
	if err != nil {
		item.Outcome = InfrastructureError
		item.Reason = "malformed protobuf stdout: " + err.Error()
		return item
	}
	if err = checkValueSizes(output, c.Limits.MaxValueBytes); err != nil {
		item.Outcome = InfrastructureError
		item.Reason = "output resource limit: " + err.Error()
		return item
	}
	if len(item.Assertions) == 0 {
		item.Outcome = Unresolved
		item.Reason = "no supported assertions"
		return item
	}
	item.Outcome = Pass
	if !compare(output, p.paths, item.Assertions) {
		item.Outcome = Fail
		item.Reason = "preservation contract violated"
	}
	return item
}

// Replay verifies the saved artifact hashes and expectations, then sends the
// exact saved input to the saved argv. It never invokes the case generator.
type ReplayOptions struct {
	Adapter    []string
	WorkingDir string
}

func Replay(dir, out string) (*Report, error) { return ReplayWithOptions(dir, out, ReplayOptions{}) }

func ReplayWithOptions(dir, out string, options ReplayOptions) (*Report, error) {
	b, err := readBounded(filepath.Join(dir, "case.json"), 8<<20)
	if err != nil {
		return nil, configError(err)
	}
	var m Manifest
	if err = decodeJSON(b, &m); err != nil {
		return nil, configError(err)
	}
	if err = m.validateContract(); err != nil {
		return nil, configError(err)
	}
	if _, ok := m.Case.Artifacts["input.bin"]; !ok {
		return nil, configError(fmt.Errorf("case has no generated input to replay"))
	}
	// Names are fixed; don't allow an evidence manifest to redirect hash reads.
	for name, want := range m.Case.Artifacts {
		limit := 16 << 20
		switch name {
		case "old.pb", "new.pb":
			limit = descriptorLimit
		case "input.bin", "output.bin", "stderr.log":
		default:
			return nil, configError(fmt.Errorf("unknown artifact name %q", name))
		}
		artifact, e := readBounded(filepath.Join(dir, name), limit)
		if e != nil {
			return nil, configError(e)
		}
		if hash(artifact) != want {
			return nil, configError(fmt.Errorf("artifact hash mismatch: %s", name))
		}
	}
	for _, name := range []string{"old.pb", "new.pb"} {
		if m.Case.Artifacts[name] == "" {
			return nil, configError(fmt.Errorf("missing descriptor hash: %s", name))
		}
	}
	c := m.Config
	if c.EmptyOutput == "" {
		c.EmptyOutput = EmptyOutputReject
	}
	if options.WorkingDir != "" {
		c.WorkingDir, err = filepath.Abs(options.WorkingDir)
		if err != nil {
			return nil, configError(err)
		}
	}
	if options.Adapter != nil {
		c.Adapter = append([]string{}, options.Adapter...)
		c.Runtime = Runtime{} // The original runtime label cannot describe a replacement.
		if len(c.Adapter) > 0 && !filepath.IsAbs(c.Adapter[0]) && strings.ContainsAny(c.Adapter[0], `/\`) {
			c.Adapter[0] = filepath.Join(c.WorkingDir, c.Adapter[0])
		}
	}
	c.OldDescriptor = filepath.Join(dir, "old.pb")
	c.NewDescriptor = filepath.Join(dir, "new.pb")
	c.Seeds = []string{filepath.Join(dir, "input.bin")}
	p, err := prepare(c)
	if err != nil {
		return nil, err
	}
	if p.unsupported == "" {
		expected := assertInput(p.seeds[0], p.paths)
		if len(expected) != len(m.Case.Assertions) {
			return nil, configError(fmt.Errorf("saved assertions do not match contract"))
		}
		for i, a := range expected {
			saved := m.Case.Assertions[i]
			if a.Path != saved.Path || !reflect.DeepEqual(a.Numbers, saved.Numbers) || a.Added != saved.Added || !reflect.DeepEqual(a.Expected, saved.Expected) {
				return nil, configError(fmt.Errorf("saved expectation differs from input: %s", a.Path))
			}
		}
	}
	if err = newEvidence(out); err != nil {
		return nil, err
	}
	pl := plan{ID: "replay", Seed: c.Seeds[0], Kind: "replay", Input: p.raw[0], Message: p.seeds[0]}
	skip := ""
	if p.unsupported != "" {
		skip = "unsupported model: " + p.unsupported
	}
	item := execute(c, p, pl, skip)
	if err = persist(filepath.Join(out, "replay"), c, p.old, p.new, p.paths, &item, pl.Input, &m.Engine); err != nil {
		return nil, err
	}
	r := &Report{Format: 2, Engine: engine(), ReplaySource: &m.Engine, Runtime: c.Runtime, Message: c.Message, Fields: p.paths, Cases: []Case{item}}
	aggregate(r)
	if err = writeJSON(filepath.Join(out, "report.json"), r); err != nil {
		return nil, err
	}
	return r, nil
}
