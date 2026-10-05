package carry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
)

type Outcome string

const (
	Pass                Outcome = "PASS"
	Fail                Outcome = "FAIL"
	Unresolved          Outcome = "UNRESOLVED"
	InfrastructureError Outcome = "INFRASTRUCTURE_ERROR"
)

type Engine struct {
	Tool     string `json:"tool"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
	Protobuf string `json:"protobuf"`
}

func engine() Engine {
	e := Engine{Version, runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH, "unknown"}
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, d := range b.Deps {
			if d.Path == "google.golang.org/protobuf" {
				e.Protobuf = d.Version
				if d.Replace != nil {
					e.Protobuf += " (replaced)"
				}
			}
		}
	}
	return e
}

type Case struct {
	ID         string            `json:"id"`
	Seed       string            `json:"seed"`
	Kind       string            `json:"kind"`
	Field      string            `json:"field,omitempty"`
	Attempted  bool              `json:"attempted"`
	Executed   bool              `json:"executed"`
	Outcome    Outcome           `json:"outcome"`
	Reason     string            `json:"reason,omitempty"`
	Evidence   string            `json:"evidence"`
	Assertions []Assertion       `json:"assertions"`
	Process    Process           `json:"process"`
	Artifacts  map[string]string `json:"sha256"`
}
type Report struct {
	Format       int             `json:"format_version"`
	Engine       Engine          `json:"engine"`
	ReplaySource *Engine         `json:"replay_source_engine,omitempty"`
	Runtime      Runtime         `json:"validation_runtime_declared"`
	Message      string          `json:"message"`
	Fields       []Path          `json:"contract"`
	Outcome      Outcome         `json:"outcome"`
	Incomplete   bool            `json:"incomplete"`
	Counts       map[Outcome]int `json:"counts"`
	Planned      int             `json:"planned_cases"`
	Attempted    int             `json:"attempted_cases"`
	Executed     int             `json:"executed_cases"`
	Cases        []Case          `json:"cases"`
}
type Manifest struct {
	Format       int     `json:"format_version"`
	Engine       Engine  `json:"engine"`
	ReplaySource *Engine `json:"replay_source_engine,omitempty"`
	Config       Config  `json:"config"`
	Contract     []Path  `json:"contract"`
	Case         Case    `json:"case"`
}

func hash(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	// Every successfully persisted case must remain readable by replay.
	if _, ok := v.(Manifest); ok && len(b) > 8<<20 {
		return fmt.Errorf("case manifest exceeds the 8 MiB evidence limit")
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func newEvidence(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("evidence directory must be new: %w", err)
	}
	return nil
}
func persist(dir string, c Config, sold, snew Schema, paths []Path, item *Case, input []byte, source *Engine) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	artifacts := map[string][]byte{"old.pb": sold.Bytes, "new.pb": snew.Bytes}
	if input != nil {
		artifacts["input.bin"] = input
	}
	if item.Process.Started {
		artifacts["output.bin"] = item.Process.Stdout
		artifacts["stderr.log"] = item.Process.Stderr
	}
	item.Artifacts = map[string]string{}
	for name, b := range artifacts {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			return err
		}
		item.Artifacts[name] = hash(b)
	}
	saved := c
	saved.OldDescriptor = "old.pb"
	saved.NewDescriptor = "new.pb"
	saved.Seeds = []string{"input.bin"}
	m := Manifest{Format: 2, Engine: engine(), ReplaySource: source, Config: saved, Contract: paths, Case: *item}
	return writeJSON(filepath.Join(dir, "case.json"), m)
}

// Compatibility is an explicit producer/contract allowlist, not a version range.
func (m Manifest) validateContract() error {
	switch {
	case m.Format == 1 && m.Engine.Tool == "0.1.0" && m.Config.EmptyOutput == "":
		return nil // Legacy evidence always rejects empty stdout.
	case m.Format == 2 && m.Engine.Tool == "0.1.1" &&
		(m.Config.EmptyOutput == EmptyOutputReject || m.Config.EmptyOutput == EmptyOutputMessage):
		return nil
	default:
		return fmt.Errorf("unsupported evidence format/producer/empty_output contract: %d/%s/%q", m.Format, m.Engine.Tool, m.Config.EmptyOutput)
	}
}

func aggregate(r *Report) {
	r.Counts = map[Outcome]int{Pass: 0, Fail: 0, Unresolved: 0, InfrastructureError: 0}
	r.Planned = len(r.Cases)
	r.Outcome = Pass
	for _, c := range r.Cases {
		r.Counts[c.Outcome]++
		if c.Attempted {
			r.Attempted++
		}
		if c.Executed {
			r.Executed++
		}
	}
	if r.Counts[Unresolved] > 0 {
		r.Outcome = Unresolved
		r.Incomplete = true
	}
	if r.Counts[Fail] > 0 {
		r.Outcome = Fail
	}
	if r.Counts[InfrastructureError] > 0 {
		r.Outcome = InfrastructureError
		r.Incomplete = true
	}
	if len(r.Cases) == 0 {
		r.Outcome = Unresolved
		r.Incomplete = true
	}
}
func ExitCode(o Outcome) int {
	switch o {
	case Pass:
		return 0
	case Fail:
		return 1
	case Unresolved:
		return 3
	default:
		return 4
	}
}
