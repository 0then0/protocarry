// Developer-only fixture regeneration, using the official descriptor builder.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/0then0/protocarry/internal/carry"
	"github.com/0then0/protocarry/internal/fixture"
)

func main() {
	save("examples/demo/old.pb", fixture.MustMarshal(fixture.Demo(false)))
	save("examples/demo/new.pb", fixture.MustMarshal(fixture.Demo(true)))
	save("examples/demo/seed.bin", fixture.DemoSeed())
	save("validation/protobufjs/old.pb", fixture.MustMarshal(fixture.External(false)))
	save("validation/protobufjs/new.pb", fixture.MustMarshal(fixture.External(true)))
	save("validation/protobufjs/seed.bin", fixture.ExternalSeed(false))
	save("validation/protobufjs/nested-seed.bin", fixture.ExternalSeed(true))
	for _, mode := range []string{"preserve", "copy", "json", "mutate"} {
		c := carry.Config{OldDescriptor: "old.pb", NewDescriptor: "new.pb", Message: "demo.Envelope", Seeds: []string{"seed.bin"}, Adapter: []string{"./bin/demo-adapter", "-schema", "examples/demo/old.pb", "-mode", mode}, WorkingDir: "../..", Fields: []string{"routing_hint", "token", "attempts", "child.future_note", "extra"}, Controls: []string{"id", "child.label"}, Limits: carry.DefaultLimits(), Runtime: carry.Runtime{Name: "google.golang.org/protobuf", Version: "v1.36.12", Options: mode}}
		b, err := json.MarshalIndent(c, "", "  ")
		must(err)
		save(filepath.Join("examples/demo", mode+".json"), append(b, '\n'))
	}
}
func save(path string, b []byte) { must(os.WriteFile(path, b, 0644)) }
func must(err error) {
	if err != nil {
		panic(err)
	}
}
