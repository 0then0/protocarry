package carry

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Schema struct {
	Root  protoreflect.MessageDescriptor
	Bytes []byte
}

func ReadSchema(path, name string) (Schema, error) {
	b, err := readBounded(path, descriptorLimit)
	if err != nil {
		return Schema{}, err
	}
	set := new(descriptorpb.FileDescriptorSet)
	if err = proto.Unmarshal(b, set); err != nil {
		return Schema{}, fmt.Errorf("descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return Schema{}, fmt.Errorf("descriptor set (include imports): %w", err)
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return Schema{}, err
	}
	root, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		return Schema{}, fmt.Errorf("%s is not a message", name)
	}
	return Schema{root, b}, nil
}

type Path struct {
	Text        string                         `json:"path"`
	Numbers     []int32                        `json:"field_numbers"`
	Added       bool                           `json:"absent_in_old_schema"`
	Fields      []protoreflect.FieldDescriptor `json:"-"`
	Unsupported string                         `json:"unsupported,omitempty"`
}

// Name/number/type preservation is checked across the reachable root graph,
// including unselected known fields. This is a scope gate, not a Buf replacement.
func CheckEvolution(old, new protoreflect.MessageDescriptor) error {
	seen := map[string]bool{}
	var walk func(protoreflect.MessageDescriptor, protoreflect.MessageDescriptor, bool) error
	walk = func(a, b protoreflect.MessageDescriptor, traversable bool) error {
		key := string(a.FullName()) + "/" + string(b.FullName()) + fmt.Sprint(traversable)
		if seen[key] {
			return nil
		}
		seen[key] = true
		if a.ParentFile().Syntax() != protoreflect.Proto3 || b.ParentFile().Syntax() != protoreflect.Proto3 {
			return fmt.Errorf("%s: only proto3 is supported", b.FullName())
		}
		if a.FullName() != b.FullName() {
			return fmt.Errorf("message type changed: %s -> %s", a.FullName(), b.FullName())
		}
		if a.Extensions().Len() > 0 || b.Extensions().Len() > 0 || a.ExtensionRanges().Len() > 0 || b.ExtensionRanges().Len() > 0 {
			return fmt.Errorf("extensions are unsupported")
		}
		for i := 0; i < a.Fields().Len(); i++ {
			x := a.Fields().Get(i)
			y := b.Fields().ByNumber(x.Number())
			if y == nil || y.Name() != x.Name() || b.Fields().ByName(x.Name()) != y {
				return fmt.Errorf("%s: existing field name/number removed or changed", x.FullName())
			}
			if x.Kind() != y.Kind() || x.Cardinality() != y.Cardinality() || x.HasPresence() != y.HasPresence() || x.IsMap() != y.IsMap() || x.IsPacked() != y.IsPacked() || x.JSONName() != y.JSONName() {
				return fmt.Errorf("%s: existing field shape changed", x.FullName())
			}
			if realOneof(x) || realOneof(y) {
				return fmt.Errorf("%s: real oneofs are unsupported", x.FullName())
			}
			if x.Kind() == protoreflect.EnumKind {
				return fmt.Errorf("%s: enum semantics are outside v0.1", x.FullName())
			}
			if x.Kind() == protoreflect.MessageKind {
				if err := walk(x.Message(), y.Message(), traversable && !x.IsList() && !x.IsMap()); err != nil {
					return err
				}
			}
		}
		for i := 0; i < b.Fields().Len(); i++ {
			y := b.Fields().Get(i)
			if a.Fields().ByNumber(y.Number()) != nil {
				continue
			}
			if a.Fields().ByName(y.Name()) != nil {
				return fmt.Errorf("%s: field number changed", y.FullName())
			}
			if !traversable || y.IsList() || y.IsMap() || realOneof(y) || y.Kind() == protoreflect.EnumKind {
				return fmt.Errorf("%s: unsupported addition (repeated/map/enum/oneof)", y.FullName())
			}
			if y.Kind() == protoreflect.MessageKind {
				if err := checkNewMessage(y.Message(), map[protoreflect.FullName]bool{}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(old, new, true)
}

func realOneof(f protoreflect.FieldDescriptor) bool {
	o := f.ContainingOneof()
	return o != nil && !o.IsSynthetic()
}

func checkNewMessage(m protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) error {
	if seen[m.FullName()] {
		return nil
	}
	seen[m.FullName()] = true
	if m.ParentFile().Syntax() != protoreflect.Proto3 || m.Extensions().Len() > 0 || m.ExtensionRanges().Len() > 0 {
		return fmt.Errorf("%s: unsupported message syntax/extensions", m.FullName())
	}
	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)
		if f.IsList() || f.IsMap() || realOneof(f) || f.Kind() == protoreflect.EnumKind {
			return fmt.Errorf("%s: unsupported added message shape", f.FullName())
		}
		if f.Kind() == protoreflect.MessageKind {
			if err := checkNewMessage(f.Message(), seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func ResolvePath(old, new protoreflect.MessageDescriptor, text string, maxDepth int) (Path, error) {
	p := Path{Text: text, Numbers: []int32{}}
	a, b := old, new
	parts := strings.Split(text, ".")
	if len(parts) > maxDepth {
		p.Unsupported = "path exceeds max_depth"
	}
	for i, name := range parts {
		f := b.Fields().ByName(protoreflect.Name(name))
		if f == nil {
			return p, fmt.Errorf("unknown field path %q at %q", text, name)
		}
		p.Fields = append(p.Fields, f)
		p.Numbers = append(p.Numbers, int32(f.Number()))
		if f.IsList() || f.IsMap() || realOneof(f) || f.Kind() == protoreflect.EnumKind {
			p.Unsupported = "repeated/map/enum/real oneof path is unsupported"
		}
		var of protoreflect.FieldDescriptor
		if a != nil {
			of = a.Fields().ByNumber(f.Number())
		}
		if of == nil {
			p.Added = true
			a = nil
		} else if of.Kind() == protoreflect.MessageKind {
			a = of.Message()
		} else {
			a = nil
		}
		if i < len(parts)-1 {
			if f.Kind() != protoreflect.MessageKind {
				return p, fmt.Errorf("%q traverses a non-message", text)
			}
			b = f.Message()
		}
	}
	last := p.Fields[len(p.Fields)-1]
	if last.Kind() == protoreflect.MessageKind {
		if err := checkSelectedMessage(last.Message(), len(parts), maxDepth, map[protoreflect.FullName]bool{}); err != nil {
			p.Unsupported = err.Error()
		}
	}
	return p, nil
}

func checkSelectedMessage(m protoreflect.MessageDescriptor, depth, max int, stack map[protoreflect.FullName]bool) error {
	if depth > max || stack[m.FullName()] {
		return fmt.Errorf("selected message exceeds max_depth or is recursive")
	}
	stack[m.FullName()] = true
	defer delete(stack, m.FullName())
	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)
		if f.IsList() || f.IsMap() || realOneof(f) || f.Kind() == protoreflect.EnumKind {
			return fmt.Errorf("selected message contains unsupported repeated/map/enum/oneof")
		}
		if depth+1 > max {
			return fmt.Errorf("selected message exceeds max_depth")
		}
		if f.Kind() == protoreflect.MessageKind {
			if err := checkSelectedMessage(f.Message(), depth+1, max, stack); err != nil {
				return err
			}
		}
	}
	return nil
}
