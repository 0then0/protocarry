package carry

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0then0/protocarry/internal/fixture"
	"github.com/0then0/protocarry/internal/relay"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestMain(m *testing.M) { _ = os.Setenv("GORACE", "atexit_sleep_ms=0"); os.Exit(m.Run()) }

// The helper is a real subprocess. Supported paths use the official old-schema
// runtime, independent of the new-schema oracle in the parent process.
func TestAdapterProcess(t *testing.T) {
	i := 0
	for i < len(os.Args) && os.Args[i] != "PROTOCARRY_HELPER" {
		i++
	}
	if i == len(os.Args) {
		return
	}
	args := os.Args[i+1:]
	mode := args[0]
	switch mode {
	case "reject":
		os.Exit(75)
	case "nonzero":
		os.Exit(42)
	case "no-read", "timeout":
		time.Sleep(time.Hour)
		os.Exit(0)
	case "early":
		_, _ = os.Stdout.Write([]byte{10, 1, 'A'})
		os.Exit(0)
	case "empty":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "malformed":
		_, _ = io.Copy(io.Discard, os.Stdin)
		_, _ = os.Stdout.Write([]byte{0xff})
		os.Exit(0)
	case "stdout-limit":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, 2<<20))
		os.Exit(0)
	case "stderr-limit":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'x'}, 2<<20))
		os.Exit(0)
	case "leaked-pipe", "detached-pipe":
		duration := "30"
		if mode == "detached-pipe" {
			duration = "1"
		}
		child := exec.Command("sleep", duration)
		if mode == "detached-pipe" {
			prepareProcess(child)
		}
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(9)
		}
		os.Exit(0)
	case "stderr-okay":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'x'}, 48<<10))
		mode = "preserve"
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(10)
	}
	s, err := ReadSchema(args[1], args[2])
	if err != nil {
		os.Exit(11)
	}
	relayMode := mode
	if mode == "reorder" || mode == "noncanonical" {
		relayMode = "preserve"
	}
	output, err := relay.Run(s.Root, input, relayMode)
	if err != nil {
		os.Exit(12)
	}
	if mode == "reorder" {
		chunks := [][]byte{}
		for len(output) > 0 {
			_, _, n := protowire.ConsumeField(output)
			if n < 0 {
				os.Exit(13)
			}
			chunks = append(chunks, output[:n])
			output = output[n:]
		}
		for i := len(chunks) - 1; i >= 0; i-- {
			output = append(output, chunks[i]...)
		}
	}
	if mode == "noncanonical" {
		// id = "A" with overlong, valid tag and length varints.
		if len(output) < 3 || !bytes.Equal(output[:3], []byte{10, 1, 'A'}) {
			os.Exit(14)
		}
		output = append([]byte{0x8a, 0x00, 0x81, 0x00, 'A'}, output[3:]...)
	}
	_, _ = os.Stdout.Write(output)
	os.Exit(0)
}

func setup(t *testing.T, mode string) Config {
	t.Helper()
	dir := t.TempDir()
	binary, err := os.Executable()
	must(t, err)
	old, new := filepath.Join(dir, "old.pb"), filepath.Join(dir, "new.pb")
	seed := filepath.Join(dir, "seed.bin")
	must(t, os.WriteFile(old, fixture.MustMarshal(fixture.Demo(false)), 0600))
	must(t, os.WriteFile(new, fixture.MustMarshal(fixture.Demo(true)), 0600))
	must(t, os.WriteFile(seed, fixture.DemoSeed(), 0600))
	return Config{OldDescriptor: old, NewDescriptor: new, Message: "demo.Envelope", Seeds: []string{seed}, WorkingDir: dir, Adapter: []string{binary, "-test.run=^TestAdapterProcess$", "--", "PROTOCARRY_HELPER", mode, old, "demo.Envelope"}, Fields: []string{"routing_hint", "token", "attempts", "child.future_note", "extra"}, Controls: []string{"id", "child.label"}, Limits: DefaultLimits()}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, c Config) (*Report, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "evidence")
	r, err := Run(c, dir)
	must(t, err)
	return r, dir
}
func outcome(t *testing.T, r *Report, want Outcome) {
	t.Helper()
	if r.Outcome != want {
		t.Fatalf("got %s, want %s; cases: %+v", r.Outcome, want, r.Cases)
	}
}
func assertion(t *testing.T, c Case, name string) Assertion {
	t.Helper()
	for _, a := range c.Assertions {
		if a.Path == name {
			return a
		}
	}
	t.Fatal("missing assertion", name)
	return Assertion{}
}

func TestPreservationModes(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want Outcome
	}{{"preserve", Pass}, {"copy", Fail}, {"json", Fail}, {"mutate", Pass}, {"reorder", Pass}, {"noncanonical", Pass}} {
		t.Run(tc.mode, func(t *testing.T) {
			c := setup(t, tc.mode)
			r, dir := check(t, c)
			outcome(t, r, tc.want)
			if r.Planned != 7 || r.Executed != 7 || r.Incomplete {
				t.Fatalf("unexpected coverage: %+v", r)
			}
			for _, name := range []string{"routing_hint", "token", "attempts", "child.future_note"} {
				a := assertion(t, r.Cases[0], name)
				if a.Preserved == nil || *a.Preserved != (tc.want == Pass) {
					t.Fatalf("%s: %+v", name, a)
				}
			}
			optional := assertion(t, r.Cases[0], "attempts")
			if optional.Expected.Value.Presence == nil || !*optional.Expected.Value.Presence || optional.Expected.Value.Value != "0" {
				t.Fatal("reference seed must have explicit zero presence")
			}
			// Values are read back by the official new runtime, not inferred from hashes.
			b, err := os.ReadFile(filepath.Join(dir, r.Cases[0].ID, "output.bin"))
			must(t, err)
			s, err := ReadSchema(c.NewDescriptor, c.Message)
			must(t, err)
			m, err := decodeMessage(s, b)
			must(t, err)
			if tc.mode == "mutate" && m.Get(m.Descriptor().Fields().ByName("score")).Int() != 42 {
				t.Fatal("permitted transform didn't run")
			}
			if tc.mode == "reorder" || tc.mode == "noncanonical" {
				if r.Cases[0].Artifacts["input.bin"] == r.Cases[0].Artifacts["output.bin"] {
					t.Fatal("wire representation should differ")
				}
			}
			if tc.want == Fail {
				nested := assertion(t, r.Cases[0], "child.future_note")
				if nested.Actual.Value.Value != "" || nested.Expected.Value.Value != "future" || !nested.Added {
					t.Fatalf("nested loss: %+v", nested)
				}
			}
		})
	}
}

func TestNumericScalars(t *testing.T) {
	kinds := []descriptorpb.FieldDescriptorProto_Type{descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED32, descriptorpb.FieldDescriptorProto_TYPE_FIXED64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64, descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, descriptorpb.FieldDescriptorProto_TYPE_BOOL}
	for _, mode := range []string{"preserve", "copy"} {
		t.Run(mode, func(t *testing.T) {
			c := setup(t, mode)
			set := fixture.Demo(true)
			c.Fields = nil
			for i, k := range kinds {
				name := strings.ToLower(strings.TrimPrefix(k.String(), "TYPE_"))
				set.File[0].MessageType[0].Field = append(set.File[0].MessageType[0].Field, fixture.Field(name, int32(i+10), k))
				c.Fields = append(c.Fields, name)
			}
			must(t, os.WriteFile(c.NewDescriptor, fixture.MustMarshal(set), 0600))
			m := dynamicpb.NewMessage(fixture.Descriptor(set, c.Message))
			must(t, proto.Unmarshal(fixture.DemoSeed(), m))
			// Boundary values exercise the official wire runtime, including 64-bit exactness.
			m.Set(m.Descriptor().Fields().ByName("int64"), protoreflect.ValueOfInt64(math.MinInt64))
			m.Set(m.Descriptor().Fields().ByName("uint64"), protoreflect.ValueOfUint64(math.MaxUint64))
			m.Set(m.Descriptor().Fields().ByName("fixed64"), protoreflect.ValueOfUint64(math.MaxUint64))
			m.Set(m.Descriptor().Fields().ByName("double"), protoreflect.ValueOfFloat64(math.Inf(1)))
			must(t, os.WriteFile(c.Seeds[0], fixture.MustMarshal(m), 0600))
			r, _ := check(t, c)
			want := Pass
			if mode == "copy" {
				want = Fail
			}
			outcome(t, r, want)
			for _, item := range r.Cases[1:] {
				a := assertion(t, item, item.Field)
				if *a.Preserved != (mode == "preserve") {
					t.Fatalf("%s: %+v", item.Field, a)
				}
			}
		})
	}
}

func TestImplicitDefaultAndOptionalPresence(t *testing.T) {
	c := setup(t, "copy")
	c.Fields = []string{"routing_hint"}
	c.Controls = []string{"id"}
	s, err := ReadSchema(c.NewDescriptor, c.Message)
	must(t, err)
	m := dynamicpb.NewMessage(s.Root)
	must(t, proto.Unmarshal(fixture.DemoSeed(), m))
	m.Clear(s.Root.Fields().ByName("routing_hint"))
	must(t, os.WriteFile(c.Seeds[0], fixture.MustMarshal(m), 0600))
	r, _ := check(t, c)
	outcome(t, r, Fail)
	if r.Cases[0].Outcome != Pass {
		t.Fatal("implicit absent/default must pass baseline")
	}
	a := assertion(t, r.Cases[0], "routing_hint")
	if a.Expected.Value.Presence != nil || a.Actual.Value.Presence != nil {
		t.Fatal("implicit scalar must not invent presence")
	}
	c.Fields = []string{"attempts"}
	r, _ = check(t, c)
	outcome(t, r, Fail)
	a = assertion(t, r.Cases[2], "attempts")
	if !*a.Expected.Value.Presence || *a.Actual.Value.Presence {
		t.Fatalf("explicit-default presence: %+v", a)
	}
	c.Adapter[4] = "preserve"
	r, _ = check(t, c)
	outcome(t, r, Pass)
}

func TestAbsentParentRequiresOptIn(t *testing.T) {
	c := setup(t, "preserve")
	c.Fields = []string{"child.future_note"}
	c.Controls = []string{"id"}
	s, err := ReadSchema(c.NewDescriptor, c.Message)
	must(t, err)
	m := dynamicpb.NewMessage(s.Root)
	must(t, proto.Unmarshal(fixture.DemoSeed(), m))
	m.Clear(s.Root.Fields().ByName("child"))
	must(t, os.WriteFile(c.Seeds[0], fixture.MustMarshal(m), 0600))
	r, _ := check(t, c)
	outcome(t, r, Unresolved)
	if r.Cases[0].Outcome != Pass || r.Cases[1].Executed {
		t.Fatal("absent parent generated without opt-in")
	}
	c.AllowCreateParents = true
	r, _ = check(t, c)
	outcome(t, r, Pass)
	if !assertion(t, r.Cases[1], "child.future_note").Expected.Parents[0] {
		t.Fatal("parent wasn't created")
	}
}

func TestLimitsAndAggregation(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want Outcome
		code int
	}{{"preserve", Unresolved, 3}, {"copy", Fail, 1}, {"nonzero", InfrastructureError, 4}} {
		t.Run(tc.mode, func(t *testing.T) {
			c := setup(t, tc.mode)
			c.Limits.MaxCases = 1
			r, _ := check(t, c)
			outcome(t, r, tc.want)
			if !r.Incomplete || r.Counts[Unresolved] != 6 || ExitCode(r.Outcome) != tc.code {
				t.Fatalf("aggregation: %+v", r)
			}
		})
	}
	c := setup(t, "preserve")
	c.Limits.MaxMessageBytes = len(fixture.DemoSeed())
	c.Limits.MaxStdoutBytes = c.Limits.MaxMessageBytes
	r, _ := check(t, c)
	if r.Counts[Unresolved] == 0 || r.Outcome == Pass {
		t.Fatal("oversize generated case falsely passed")
	}
}

func TestSubprocessFailures(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want Outcome
	}{{"reject", Unresolved}, {"nonzero", InfrastructureError}, {"empty", InfrastructureError}, {"malformed", InfrastructureError}, {"stderr-okay", Pass}, {"stdout-limit", InfrastructureError}, {"stderr-limit", InfrastructureError}, {"no-read", InfrastructureError}, {"timeout", InfrastructureError}, {"leaked-pipe", InfrastructureError}, {"detached-pipe", InfrastructureError}} {
		for _, policy := range []string{EmptyOutputReject, EmptyOutputMessage} {
			t.Run(tc.mode+"/"+policy, func(t *testing.T) {
				c := setup(t, tc.mode)
				c.EmptyOutput = policy
				c.Fields = []string{"routing_hint"}
				c.Controls = []string{"id"}
				c.Limits.MaxCases = 1
				if tc.mode == "no-read" || tc.mode == "timeout" {
					c.Limits.TimeoutMS = 100
				}
				r, _ := check(t, c)
				want := tc.want
				if (tc.mode == "empty" || tc.mode == "leaked-pipe") && policy == EmptyOutputMessage {
					want = Fail
				}
				if r.Cases[0].Outcome != want {
					t.Fatalf("got %+v", r.Cases[0])
				}
				if tc.mode == "stdout-limit" && !r.Cases[0].Process.StdoutTruncated {
					t.Fatal("missing stdout truncation")
				}
				if tc.mode == "stderr-limit" && !r.Cases[0].Process.StderrTruncated {
					t.Fatal("missing stderr truncation")
				}
				if tc.mode == "timeout" && !r.Cases[0].Process.Timeout {
					t.Fatal("missing timeout")
				}
				if tc.mode == "detached-pipe" && r.Cases[0].Reason != "incomplete pipe drain (possibly an inherited pipe)" {
					t.Fatal("detached pipe did not exercise incomplete drain", r.Cases[0])
				}
			})
		}
	}
	t.Run("missing-executable", func(t *testing.T) {
		c := setup(t, "preserve")
		c.Adapter = []string{filepath.Join(c.WorkingDir, "absent")}
		r, _ := check(t, c)
		outcome(t, r, InfrastructureError)
		if r.Executed != 0 || r.Attempted == 0 {
			t.Fatal("start failure counts")
		}
	})
	t.Run("partial-stdin", func(t *testing.T) {
		c := setup(t, "early")
		c.EmptyOutput = EmptyOutputMessage
		r := runProcess(c, bytes.Repeat([]byte{'x'}, 1<<20))
		if r.Error != "adapter exited before receiving complete input" || r.InputBytesWritten >= 1<<20 {
			t.Fatalf("partial input: %+v", r)
		}
	})
	t.Run("blocked-stdin", func(t *testing.T) {
		c := setup(t, "no-read")
		c.EmptyOutput = EmptyOutputMessage
		c.Limits.TimeoutMS = 100
		start := time.Now()
		r := runProcess(c, bytes.Repeat([]byte{'x'}, 1<<20))
		if !r.Timeout || time.Since(start) > 2*time.Second {
			t.Fatal("blocked stdin didn't stop")
		}
	})
}

func TestReportStableAndReplay(t *testing.T) {
	c := setup(t, "copy")
	r, dir := check(t, c)
	outcome(t, r, Fail)
	r2, dir2 := check(t, c)
	outcome(t, r2, Fail)
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	must(t, err)
	b2, err := os.ReadFile(filepath.Join(dir2, "report.json"))
	must(t, err)
	if !bytes.Equal(b, b2) {
		t.Fatal("successful executions produced unstable report")
	}
	caseDir := filepath.Join(dir, "s001-baseline")
	replayed := filepath.Join(t.TempDir(), "replayed")
	rr, err := Replay(caseDir, replayed)
	must(t, err)
	outcome(t, rr, Fail)
	if rr.Planned != 1 || rr.Cases[0].Artifacts["input.bin"] != r.Cases[0].Artifacts["input.bin"] {
		t.Fatal("replay regenerated input")
	}
	// Corrupt evidence must be rejected before invoking the subprocess.
	must(t, os.WriteFile(filepath.Join(caseDir, "input.bin"), []byte{10, 1, 'X'}, 0600))
	_, err = Replay(caseDir, filepath.Join(t.TempDir(), "bad"))
	var ce *ConfigError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatal("tampered input accepted", err)
	}
}

func TestInvalidSeedAndImportPreflight(t *testing.T) {
	c := setup(t, "preserve")
	must(t, os.WriteFile(c.Seeds[0], []byte{0xff}, 0600))
	_, err := Run(c, filepath.Join(t.TempDir(), "out"))
	var ce *ConfigError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "invalid seed") {
		t.Fatal(err)
	}
	c = setup(t, "preserve")
	set := fixture.Demo(false)
	set.File[0].Dependency = []string{"missing.proto"}
	must(t, os.WriteFile(c.OldDescriptor, fixture.MustMarshal(set), 0600))
	_, err = Run(c, filepath.Join(t.TempDir(), "out"))
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "imports") {
		t.Fatal(err)
	}
	c = setup(t, "preserve")
	c.Fields = []string{"typo"}
	_, err = Run(c, filepath.Join(t.TempDir(), "out"))
	if !errors.As(err, &ce) {
		t.Fatal("invalid path accepted")
	}
}

func TestUnsupportedEvolutionStopsBeforeAdapter(t *testing.T) {
	for _, change := range []string{"type", "number", "rename", "oneof", "enum", "proto2", "editions", "repeated-addition", "nested-type"} {
		t.Run(change, func(t *testing.T) {
			c := setup(t, "preserve")
			set := fixture.Demo(true)
			root := set.File[0].MessageType[0]
			switch change {
			case "type":
				root.Field[0].Type = descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()
			case "number":
				root.Field[0].Number = proto.Int32(8)
			case "rename":
				root.Field[0].Name = proto.String("identifier")
				c.Controls = []string{"child.label"}
			case "oneof":
				root.OneofDecl = append([]*descriptorpb.OneofDescriptorProto{{Name: proto.String("real")}}, root.OneofDecl...)
				root.Field[0].OneofIndex = proto.Int32(0)
				for _, f := range root.Field {
					if f.GetProto3Optional() {
						f.OneofIndex = proto.Int32(1)
					}
				}
			case "enum":
				set.File[0].EnumType = []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Status"), Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("ZERO"), Number: proto.Int32(0)}}}}
				root.Field = append(root.Field, fixture.Field("status", 9, descriptorpb.FieldDescriptorProto_TYPE_ENUM))
				root.Field[len(root.Field)-1].TypeName = proto.String(".demo.Status")
			case "proto2":
				old := fixture.Demo(false)
				old.File[0].Syntax = proto.String("proto2")
				must(t, os.WriteFile(c.OldDescriptor, fixture.MustMarshal(old), 0600))
			case "editions":
				old := fixture.Demo(false)
				old.File[0].Syntax = proto.String("editions")
				old.File[0].Edition = descriptorpb.Edition_EDITION_2023.Enum()
				must(t, os.WriteFile(c.OldDescriptor, fixture.MustMarshal(old), 0600))
			case "repeated-addition":
				f := fixture.Field("many", 9, descriptorpb.FieldDescriptorProto_TYPE_INT32)
				f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
				root.Field = append(root.Field, f)
			case "nested-type":
				set.File[0].MessageType[1].Field[0].Type = descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()
			}
			must(t, os.WriteFile(c.NewDescriptor, fixture.MustMarshal(set), 0600))
			r, _ := check(t, c)
			outcome(t, r, Unresolved)
			if r.Executed != 0 || r.Attempted != 0 {
				t.Fatal("unsupported model launched adapter")
			}
		})
	}
}

func TestDepthAndSelectionScope(t *testing.T) {
	c := setup(t, "preserve")
	c.Limits.MaxDepth = 1
	r, _ := check(t, c)
	outcome(t, r, Unresolved)
	if r.Executed != 0 {
		t.Fatal("deep path checked")
	}
	c = setup(t, "preserve")
	c.Fields = []string{"id"}
	_, err := Run(c, filepath.Join(t.TempDir(), "out"))
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatal("existing field accepted as addition")
	}
}

func TestFloatSemanticsAgainstOfficialRuntime(t *testing.T) {
	set := fixture.Demo(true)
	root := set.File[0].MessageType[0]
	root.Field = append(root.Field, fixture.Field("f", 9, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE))
	d := fixture.Descriptor(set, "demo.Envelope")
	f := d.Fields().ByName("f")
	a, b := dynamicpb.NewMessage(d), dynamicpb.NewMessage(d)
	for _, pair := range [][2]float64{{0, math.Copysign(0, -1)}, {math.NaN(), math.Float64frombits(0x7ff8000000000001)}, {math.Inf(1), math.Inf(1)}, {1.25, 1.25}} {
		a.Set(f, protoreflect.ValueOfFloat64(pair[0]))
		b.Set(f, protoreflect.ValueOfFloat64(pair[1]))
		// proto.Equal may distinguish implicit +0 (not populated) from -0
		// (populated). The contract explicitly compares numeric scalar values,
		// and uses presence only for fields with descriptor-defined presence.
		if pair[0] != 0 && !proto.Equal(a, b) {
			t.Fatal("official runtime equality assumption changed", pair)
		}
		da, db := dynamicpb.NewMessage(d), dynamicpb.NewMessage(d)
		must(t, proto.Unmarshal(fixture.MustMarshal(a), da))
		must(t, proto.Unmarshal(fixture.MustMarshal(b), db))
		a, b = da, db
		p := Path{Text: "f", Fields: []protoreflect.FieldDescriptor{f}, Numbers: []int32{9}}
		as := assertInput(a, []Path{p})
		if !compare(b, []Path{p}, as) {
			t.Fatal("oracle disagreed with official scalar equality")
		}
	}
	a.Set(f, protoreflect.ValueOfFloat64(1))
	b.Set(f, protoreflect.ValueOfFloat64(math.Nextafter(1, 2)))
	p := Path{Text: "f", Fields: []protoreflect.FieldDescriptor{f}}
	if compare(b, []Path{p}, assertInput(a, []Path{p})) {
		t.Fatal("floats should have no implicit tolerance")
	}
}

func TestControlMutationIsAnAssertion(t *testing.T) {
	c := setup(t, "mutate")
	c.Controls = append(c.Controls, "score")
	r, _ := check(t, c)
	outcome(t, r, Fail)
	a := assertion(t, r.Cases[0], "score")
	if a.Expected.Value.Value != "0" || a.Actual.Value.Value != "42" || *a.Preserved {
		t.Fatalf("control mutation: %+v", a)
	}
	if !*assertion(t, r.Cases[0], "routing_hint").Preserved {
		t.Fatal("permitted new field did not survive")
	}
}

func TestUnselectedCollectionsAndNestedScopeGate(t *testing.T) {
	c := setup(t, "preserve")
	old, new := fixture.Demo(false), fixture.Demo(true)
	for _, set := range []*descriptorpb.FileDescriptorSet{old, new} {
		root := set.File[0].MessageType[0]
		list := fixture.Field("numbers", 10, descriptorpb.FieldDescriptorProto_TYPE_INT32)
		list.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
		entry := &descriptorpb.DescriptorProto{Name: proto.String("LabelsEntry"), Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)}, Field: []*descriptorpb.FieldDescriptorProto{fixture.Field("key", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING), fixture.Field("value", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32)}}
		root.NestedType = []*descriptorpb.DescriptorProto{entry}
		labels := fixture.MsgField("labels", 11, ".demo.Envelope.LabelsEntry")
		labels.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
		root.Field = append(root.Field, list, labels)
	}
	must(t, os.WriteFile(c.OldDescriptor, fixture.MustMarshal(old), 0600))
	must(t, os.WriteFile(c.NewDescriptor, fixture.MustMarshal(new), 0600))
	m := dynamicpb.NewMessage(fixture.Descriptor(new, c.Message))
	must(t, proto.Unmarshal(fixture.DemoSeed(), m))
	m.Mutable(m.Descriptor().Fields().ByName("numbers")).List().Append(protoreflect.ValueOfInt32(123))
	m.Mutable(m.Descriptor().Fields().ByName("labels")).Map().Set(protoreflect.ValueOfString("seed").MapKey(), protoreflect.ValueOfInt32(2))
	must(t, os.WriteFile(c.Seeds[0], fixture.MustMarshal(m), 0600))
	r, _ := check(t, c)
	outcome(t, r, Pass)
	c.Controls = append(c.Controls, "numbers")
	r, _ = check(t, c)
	outcome(t, r, Unresolved)
	if r.Executed > 0 {
		t.Fatal("collection assertion must stop dependent checks")
	}
	// Evolution beneath a repeated message is unsupported even if unselected.
	c = setup(t, "preserve")
	c.Fields = []string{"routing_hint"}
	c.Controls = []string{"id"}
	old, new = fixture.Demo(false), fixture.Demo(true)
	for _, set := range []*descriptorpb.FileDescriptorSet{old, new} {
		set.File[0].MessageType[0].Field[1].Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	}
	must(t, os.WriteFile(c.OldDescriptor, fixture.MustMarshal(old), 0600))
	must(t, os.WriteFile(c.NewDescriptor, fixture.MustMarshal(new), 0600))
	r, _ = check(t, c)
	outcome(t, r, Unresolved)
	if r.Executed > 0 {
		t.Fatal("nested repeated evolution checked")
	}
}

func TestOptionalMessageHasNoScalarDefaultVariant(t *testing.T) {
	for _, mode := range []string{"preserve", "copy"} {
		t.Run(mode, func(t *testing.T) {
			c := setup(t, mode)
			c.Fields = []string{"extra"}
			c.Controls = []string{"id"}
			set := fixture.Demo(true)
			root := set.File[0].MessageType[0]
			root.OneofDecl = append(root.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: proto.String("_extra")})
			for _, f := range root.Field {
				if f.GetName() == "extra" {
					f.Proto3Optional = proto.Bool(true)
					f.OneofIndex = proto.Int32(1)
				}
			}
			must(t, os.WriteFile(c.NewDescriptor, fixture.MustMarshal(set), 0600))
			r, _ := check(t, c)
			want := Pass
			if mode == "copy" {
				want = Fail
			}
			outcome(t, r, want)
			if r.Planned != 2 || r.Executed != 2 {
				t.Fatal("messages do not have scalar explicit-default variants")
			}
		})
	}
}
