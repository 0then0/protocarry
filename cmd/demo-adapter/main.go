// A minimal binary adapter for the example application paths.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/0then0/protocarry/internal/carry"
	"github.com/0then0/protocarry/internal/relay"
)

func main() {
	schema := flag.String("schema", "examples/demo/old.pb", "old descriptor set")
	message := flag.String("message", "demo.Envelope", "root message")
	mode := flag.String("mode", "preserve", "preserve, copy, json, mutate")
	flag.Parse()
	s, err := carry.ReadSchema(*schema, *message)
	if err != nil {
		die(err)
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, (16<<20)+1))
	if err != nil {
		die(err)
	}
	if len(input) > 16<<20 {
		die(fmt.Errorf("input too large"))
	}
	output, err := relay.Run(s.Root, input, *mode)
	if err != nil {
		die(err)
	}
	if _, err = os.Stdout.Write(output); err != nil {
		die(err)
	}
}
func die(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
