// Package carry implements the bounded ProtoCarry preservation contract.
package carry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Version = "0.1.1"
const descriptorLimit = 8 << 20

const (
	EmptyOutputReject  = "reject"
	EmptyOutputMessage = "message"
)

type Limits struct {
	TimeoutMS       int `json:"timeout_ms"`
	MaxCases        int `json:"max_cases"`
	MaxDepth        int `json:"max_depth"`
	MaxMessageBytes int `json:"max_message_bytes"`
	MaxValueBytes   int `json:"max_value_bytes"`
	MaxStdoutBytes  int `json:"max_stdout_bytes"`
	MaxStderrBytes  int `json:"max_stderr_bytes"`
}

// Runtime is a user-supplied label, not a claim independently verified by the CLI.
type Runtime struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Options string `json:"options"`
}

type Config struct {
	OldDescriptor      string   `json:"old_descriptor"`
	NewDescriptor      string   `json:"new_descriptor"`
	Message            string   `json:"message"`
	Seeds              []string `json:"seeds"`
	Adapter            []string `json:"adapter"`
	WorkingDir         string   `json:"working_dir"`
	Fields             []string `json:"fields"`
	Controls           []string `json:"controls"`
	AllowCreateParents bool     `json:"allow_create_parents"`
	EmptyOutput        string   `json:"empty_output,omitempty"`
	Limits             Limits   `json:"limits"`
	Runtime            Runtime  `json:"validation_runtime"`
}

func DefaultLimits() Limits {
	return Limits{2000, 64, 8, 1 << 20, 64, 1 << 20, 64 << 10}
}

func decodeJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}

func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := readBounded(path, 1<<20)
	if err != nil {
		return c, err
	}
	// Defaults also apply when only some limit keys are provided. Explicit zero is invalid.
	c.Limits = DefaultLimits()
	c.EmptyOutput = EmptyOutputReject
	// Keep the raw option so an omitted key defaults, but explicit null is invalid.
	wire := struct {
		*Config
		EmptyOutput json.RawMessage `json:"empty_output"`
	}{Config: &c}
	if err := decodeJSON(b, &wire); err != nil {
		return c, err
	}
	if len(wire.EmptyOutput) > 0 {
		c.EmptyOutput = ""
		if err := json.Unmarshal(wire.EmptyOutput, &c.EmptyOutput); err != nil {
			return c, fmt.Errorf("empty_output must be reject or message: %w", err)
		}
	}
	if c.EmptyOutput == "" {
		return c, fmt.Errorf("empty_output must be reject or message")
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	c.OldDescriptor = resolve(c.OldDescriptor)
	c.NewDescriptor = resolve(c.NewDescriptor)
	for i := range c.Seeds {
		c.Seeds[i] = resolve(c.Seeds[i])
	}
	if c.WorkingDir == "" {
		c.WorkingDir = base
	} else {
		c.WorkingDir = resolve(c.WorkingDir)
	}
	// Relative executable paths belong to working_dir; bare names use PATH.
	if len(c.Adapter) > 0 && !filepath.IsAbs(c.Adapter[0]) && strings.ContainsAny(c.Adapter[0], `/\`) {
		c.Adapter[0] = filepath.Join(c.WorkingDir, c.Adapter[0])
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.EmptyOutput != "" && c.EmptyOutput != EmptyOutputReject && c.EmptyOutput != EmptyOutputMessage {
		return fmt.Errorf("empty_output must be reject or message")
	}
	if c.OldDescriptor == "" || c.NewDescriptor == "" || c.Message == "" {
		return fmt.Errorf("old_descriptor, new_descriptor and message are required")
	}
	if len(c.Seeds) == 0 || len(c.Seeds) > 128 {
		return fmt.Errorf("seeds must contain 1..128 paths")
	}
	if len(c.Adapter) == 0 || c.Adapter[0] == "" {
		return fmt.Errorf("adapter must be a nonempty argv array")
	}
	for _, a := range c.Adapter {
		if strings.ContainsRune(a, 0) {
			return fmt.Errorf("adapter argv contains NUL")
		}
	}
	if len(c.Fields) == 0 || len(c.Fields)+len(c.Controls) > 128 {
		return fmt.Errorf("fields must be nonempty; at most 128 contract paths")
	}
	seen := map[string]bool{}
	for _, p := range append(append([]string{}, c.Fields...), c.Controls...) {
		if p == "" || seen[p] {
			return fmt.Errorf("empty or duplicate contract path %q", p)
		}
		seen[p] = true
	}
	l := c.Limits
	if l.TimeoutMS < 1 || l.TimeoutMS > 60000 || l.MaxCases < 1 || l.MaxCases > 1000 || l.MaxDepth < 1 || l.MaxDepth > 32 || l.MaxMessageBytes < 1 || l.MaxMessageBytes > 16<<20 || l.MaxValueBytes < 1 || l.MaxValueBytes > 4096 || l.MaxStdoutBytes < 1 || l.MaxStdoutBytes > 16<<20 || l.MaxStderrBytes < 1 || l.MaxStderrBytes > 1<<20 {
		return fmt.Errorf("limits outside v0.1 bounds (see docs/contract.md)")
	}
	if c.Limits.MaxStdoutBytes > c.Limits.MaxMessageBytes {
		return fmt.Errorf("max_stdout_bytes must be <= max_message_bytes")
	}
	st, err := os.Stat(c.WorkingDir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("working_dir must be an existing directory: %s", c.WorkingDir)
	}
	return nil
}

func readBounded(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return b, nil
}
