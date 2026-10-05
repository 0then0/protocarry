package carry

import (
	"encoding/base64"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Value struct {
	Kind     string   `json:"kind"`
	Presence *bool    `json:"presence,omitempty"`
	Value    string   `json:"value"`
	Fields   []Member `json:"fields,omitempty"`
}
type Member struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
	Value  Value  `json:"value"`
}
type Observation struct {
	Parents []bool `json:"parent_presence"`
	Value   Value  `json:"value"`
}
type Assertion struct {
	Path      string       `json:"path"`
	Numbers   []int32      `json:"field_numbers"`
	Added     bool         `json:"absent_in_old_schema"`
	Expected  Observation  `json:"expected"`
	Actual    *Observation `json:"actual,omitempty"`
	Preserved *bool        `json:"preserved,omitempty"`
}

func decodeMessage(s Schema, b []byte) (*dynamicpb.Message, error) {
	m := dynamicpb.NewMessage(s.Root)
	err := (proto.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(b, m)
	return m, err
}

func observe(m protoreflect.Message, p Path) Observation {
	o := Observation{Parents: []bool{}}
	for _, f := range p.Fields[:len(p.Fields)-1] {
		has := m.Has(f)
		o.Parents = append(o.Parents, has)
		if has {
			m = m.Get(f).Message()
		} else {
			m = dynamicpb.NewMessage(f.Message())
		}
	}
	o.Value = fieldValue(m, p.Fields[len(p.Fields)-1])
	return o
}

func fieldValue(m protoreflect.Message, f protoreflect.FieldDescriptor) Value {
	v := Value{Kind: f.Kind().String()}
	if f.HasPresence() {
		p := m.Has(f)
		v.Presence = &p
	}
	x := m.Get(f)
	switch f.Kind() {
	case protoreflect.MessageKind:
		if !m.Has(f) {
			return v
		}
		child := x.Message()
		fields := make([]protoreflect.FieldDescriptor, child.Descriptor().Fields().Len())
		for i := range fields {
			fields[i] = child.Descriptor().Fields().Get(i)
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Number() < fields[j].Number() })
		for _, cf := range fields {
			v.Fields = append(v.Fields, Member{string(cf.Name()), int32(cf.Number()), fieldValue(child, cf)})
		}
	case protoreflect.StringKind:
		v.Value = x.String()
	case protoreflect.BytesKind:
		v.Value = base64.StdEncoding.EncodeToString(x.Bytes())
	case protoreflect.BoolKind:
		v.Value = strconv.FormatBool(x.Bool())
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		n := x.Float()
		if n == 0 {
			v.Value = "0"
		} else if math.IsNaN(n) {
			v.Value = "NaN"
		} else {
			v.Value = strconv.FormatFloat(n, 'g', -1, 64)
		}
	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		v.Value = strconv.FormatInt(x.Int(), 10)
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		v.Value = strconv.FormatUint(x.Uint(), 10)
	}
	return v
}

func assertInput(m protoreflect.Message, paths []Path) []Assertion {
	out := make([]Assertion, 0, len(paths))
	for _, p := range paths {
		if p.Unsupported == "" {
			out = append(out, Assertion{Path: p.Text, Numbers: p.Numbers, Added: p.Added, Expected: observe(m, p)})
		}
	}
	return out
}
func compare(output protoreflect.Message, paths []Path, as []Assertion) bool {
	byName := map[string]Path{}
	for _, p := range paths {
		byName[p.Text] = p
	}
	all := true
	for i := range as {
		o := observe(output, byName[as[i].Path])
		same := reflect.DeepEqual(as[i].Expected, o)
		as[i].Actual = &o
		as[i].Preserved = &same
		all = all && same
	}
	return all
}

// Validate all descriptor-known strings/bytes, including opaque collections.
func checkValueSizes(m protoreflect.Message, limit int) error {
	var problem error
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		check := func(fd protoreflect.FieldDescriptor, x protoreflect.Value) error {
			switch fd.Kind() {
			case protoreflect.StringKind:
				if len(x.String()) > limit {
					return fmt.Errorf("%s: string exceeds max_value_bytes", fd.FullName())
				}
			case protoreflect.BytesKind:
				if len(x.Bytes()) > limit {
					return fmt.Errorf("%s: bytes exceed max_value_bytes", fd.FullName())
				}
			case protoreflect.MessageKind:
				return checkValueSizes(x.Message(), limit)
			}
			return nil
		}
		if f.IsMap() {
			v.Map().Range(func(k protoreflect.MapKey, x protoreflect.Value) bool {
				problem = check(f.MapKey(), k.Value())
				if problem == nil {
					problem = check(f.MapValue(), x)
				}
				return problem == nil
			})
		} else if f.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				problem = check(f, v.List().Get(i))
				if problem != nil {
					break
				}
			}
		} else {
			problem = check(f, v)
		}
		return problem == nil
	})
	return problem
}
