// Package fixture defines the small demo schemas using official descriptor types.
// This developer helper is not a .proto parser and is not used by ProtoCarry.
package fixture

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func Field(name string, n int32, kind descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(n), Type: kind.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}
}
func MsgField(name string, n int32, typ string) *descriptorpb.FieldDescriptorProto {
	f := Field(name, n, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE)
	f.TypeName = proto.String(typ)
	return f
}
func Demo(new bool) *descriptorpb.FileDescriptorSet {
	child := &descriptorpb.DescriptorProto{Name: proto.String("Child"), Field: []*descriptorpb.FieldDescriptorProto{Field("label", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)}}
	root := &descriptorpb.DescriptorProto{Name: proto.String("Envelope"), Field: []*descriptorpb.FieldDescriptorProto{Field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING), MsgField("child", 5, ".demo.Child"), Field("score", 7, descriptorpb.FieldDescriptorProto_TYPE_INT32)}}
	messages := []*descriptorpb.DescriptorProto{root, child}
	if new {
		child.Field = append(child.Field, Field("future_note", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING))
		opt := Field("attempts", 4, descriptorpb.FieldDescriptorProto_TYPE_INT32)
		opt.Proto3Optional = proto.Bool(true)
		opt.OneofIndex = proto.Int32(0)
		root.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("_attempts")}}
		root.Field = append(root.Field, Field("routing_hint", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING), Field("token", 3, descriptorpb.FieldDescriptorProto_TYPE_BYTES), opt, MsgField("extra", 6, ".demo.Future"))
		messages = append(messages, &descriptorpb.DescriptorProto{Name: proto.String("Future"), Field: []*descriptorpb.FieldDescriptorProto{Field("note", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)}})
	}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("demo.proto"), Package: proto.String("demo"), Syntax: proto.String("proto3"), MessageType: messages}}}
}
func External(new bool) *descriptorpb.FileDescriptorSet {
	child := &descriptorpb.DescriptorProto{Name: proto.String("Child"), Field: []*descriptorpb.FieldDescriptorProto{Field("label", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)}}
	root := &descriptorpb.DescriptorProto{Name: proto.String("Envelope"), Field: []*descriptorpb.FieldDescriptorProto{Field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)}}
	nested := &descriptorpb.DescriptorProto{Name: proto.String("NestedEnvelope"), Field: []*descriptorpb.FieldDescriptorProto{Field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING), MsgField("child", 3, ".validation.Child")}}
	if new {
		root.Field = append(root.Field, Field("future_note", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING))
		child.Field = append(child.Field, Field("future_note", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING))
	}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("validation.proto"), Package: proto.String("validation"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{root, nested, child}}}}
}
func Descriptor(set *descriptorpb.FileDescriptorSet, name string) protoreflect.MessageDescriptor {
	files, err := protodesc.NewFiles(set)
	if err != nil {
		panic(err)
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		panic(err)
	}
	return d.(protoreflect.MessageDescriptor)
}
func DemoSeed() []byte {
	m := dynamicpb.NewMessage(Descriptor(Demo(true), "demo.Envelope"))
	m.Set(m.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString("A"))
	m.Set(m.Descriptor().Fields().ByName("routing_hint"), protoreflect.ValueOfString("future-route"))
	m.Set(m.Descriptor().Fields().ByName("token"), protoreflect.ValueOfBytes([]byte{0, 1, 2}))
	m.Set(m.Descriptor().Fields().ByName("attempts"), protoreflect.ValueOfInt32(0))
	child := m.Mutable(m.Descriptor().Fields().ByName("child")).Message()
	child.Set(child.Descriptor().Fields().ByName("label"), protoreflect.ValueOfString("seed"))
	child.Set(child.Descriptor().Fields().ByName("future_note"), protoreflect.ValueOfString("future"))
	return MustMarshal(m)
}
func ExternalSeed(nested bool) []byte {
	name := "validation.Envelope"
	if nested {
		name = "validation.NestedEnvelope"
	}
	m := dynamicpb.NewMessage(Descriptor(External(true), name))
	m.Set(m.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString("A"))
	if nested {
		child := m.Mutable(m.Descriptor().Fields().ByName("child")).Message()
		child.Set(child.Descriptor().Fields().ByName("label"), protoreflect.ValueOfString("seed"))
		child.Set(child.Descriptor().Fields().ByName("future_note"), protoreflect.ValueOfString("future"))
	} else {
		m.Set(m.Descriptor().Fields().ByName("future_note"), protoreflect.ValueOfString("future"))
	}
	return MustMarshal(m)
}
func MustMarshal(m proto.Message) []byte {
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}
