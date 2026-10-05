// Package relay is the demo application path, separate from the oracle.
package relay

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func Run(d protoreflect.MessageDescriptor, input []byte, mode string) ([]byte, error) {
	m := dynamicpb.NewMessage(d)
	if err := proto.Unmarshal(input, m); err != nil {
		return nil, err
	}
	switch mode {
	case "preserve":
	case "copy":
		m = copyKnown(m.ProtoReflect()).Interface().(*dynamicpb.Message)
	case "json":
		b, err := protojson.Marshal(m)
		if err != nil {
			return nil, err
		}
		m = dynamicpb.NewMessage(d)
		if err = protojson.Unmarshal(b, m); err != nil {
			return nil, err
		}
	case "mutate":
		f := d.Fields().ByName("score")
		if f == nil {
			return nil, fmt.Errorf("mutate requires score field")
		}
		m.Set(f, protoreflect.ValueOfInt32(42))
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
	return (proto.MarshalOptions{Deterministic: true}).Marshal(m)
}
func copyKnown(src protoreflect.Message) protoreflect.Message {
	dst := dynamicpb.NewMessage(src.Descriptor())
	src.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.Kind() == protoreflect.MessageKind {
			dst.Set(f, protoreflect.ValueOfMessage(copyKnown(v.Message())))
		} else {
			dst.Set(f, v)
		}
		return true
	})
	return dst
}
