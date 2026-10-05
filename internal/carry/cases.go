package carry

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type plan struct {
	ID, Seed, Kind, Field, Reason string
	Input                         []byte
	Message                       *dynamicpb.Message
}

func makePlans(c Config, seeds []*dynamicpb.Message, raw [][]byte, paths []Path) []plan {
	out := []plan{}
	for i, m := range seeds {
		out = append(out, plan{ID: fmt.Sprintf("s%03d-baseline", i+1), Seed: c.Seeds[i], Kind: "baseline", Input: raw[i], Message: m})
	}
	for i, seed := range seeds {
		for j, p := range paths[:len(c.Fields)] {
			variants := []string{"value"}
			last := p.Fields[len(p.Fields)-1]
			if last.HasOptionalKeyword() && last.Kind() != protoreflect.MessageKind {
				variants = append(variants, "explicit-default")
			}
			for _, variant := range variants {
				item := plan{ID: fmt.Sprintf("s%03d-f%03d-%s", i+1, j+1, variant), Seed: c.Seeds[i], Kind: variant, Field: p.Text}
				if p.Unsupported != "" {
					item.Reason = p.Unsupported
					out = append(out, item)
					continue
				}
				item.Message = seed
				out = append(out, item)
			}
		}
	}
	return out
}

func generate(c Config, s Schema, item plan, p Path) plan {
	m := proto.Clone(item.Message).(*dynamicpb.Message)
	target := m.ProtoReflect()
	for _, f := range p.Fields[:len(p.Fields)-1] {
		if !target.Has(f) && !c.AllowCreateParents {
			item.Reason = "absent parent: set allow_create_parents to permit material seed changes"
			item.Message = nil
			return item
		}
		target = target.Mutable(f).Message()
	}
	last := p.Fields[len(p.Fields)-1]
	if item.Kind == "explicit-default" {
		target.Set(last, last.Default())
	} else {
		setDistinct(target, last, c.Limits.MaxValueBytes)
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(m)
	if err != nil {
		item.Reason = "generator could not marshal: " + err.Error()
	} else if len(b) > c.Limits.MaxMessageBytes {
		item.Reason = "generated input exceeds max_message_bytes"
	} else {
		verified, e := decodeMessage(s, b)
		if e != nil {
			item.Reason = "generator produced invalid input: " + e.Error()
		} else {
			item.Input = b
			item.Message = verified
			return item
		}
	}
	item.Message = nil
	return item
}

func setDistinct(m protoreflect.Message, f protoreflect.FieldDescriptor, limit int) {
	switch f.Kind() {
	case protoreflect.MessageKind:
		child := m.Mutable(f).Message()
		// The selected new message is a unit of the contract. Mutate one leaf only.
		for i := 0; i < f.Message().Fields().Len(); i++ {
			cf := f.Message().Fields().Get(i)
			if cf.Kind() != protoreflect.MessageKind {
				setDistinct(child, cf, limit)
				return
			}
		}
		if f.Message().Fields().Len() > 0 {
			setDistinct(child, f.Message().Fields().Get(0), limit)
		}
	case protoreflect.StringKind:
		x := "protocarry"
		if len(x) > limit {
			x = x[:limit]
		}
		if m.Get(f).String() == x {
			x = strings.Repeat("q", len(x))
		}
		m.Set(f, protoreflect.ValueOfString(x))
	case protoreflect.BytesKind:
		x := []byte{1, 112, 99}
		if len(x) > limit {
			x = x[:limit]
		}
		if string(m.Get(f).Bytes()) == string(x) {
			x[0] = 2
		}
		m.Set(f, protoreflect.ValueOfBytes(x))
	case protoreflect.BoolKind:
		m.Set(f, protoreflect.ValueOfBool(true))
	case protoreflect.FloatKind:
		m.Set(f, protoreflect.ValueOfFloat32(1.25))
	case protoreflect.DoubleKind:
		m.Set(f, protoreflect.ValueOfFloat64(2.5))
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		x := int32(-37)
		if m.Get(f).Int() == int64(x) {
			x = -38
		}
		m.Set(f, protoreflect.ValueOfInt32(x))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		x := int64(-37)
		if m.Get(f).Int() == x {
			x = -38
		}
		m.Set(f, protoreflect.ValueOfInt64(x))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		x := uint32(37)
		if m.Get(f).Uint() == uint64(x) {
			x = 38
		}
		m.Set(f, protoreflect.ValueOfUint32(x))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		x := uint64(37)
		if m.Get(f).Uint() == x {
			x = 38
		}
		m.Set(f, protoreflect.ValueOfUint64(x))
	}
}
