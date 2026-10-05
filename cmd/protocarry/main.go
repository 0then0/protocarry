package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/0then0/protocarry/internal/carry"
)

func main() { os.Exit(cli(os.Args[1:], os.Stdout, os.Stderr)) }
func cli(args []string, out, logs io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, "ProtoCarry "+carry.Version)
		return 0
	}
	if len(args) == 0 || (args[0] != "check" && args[0] != "replay") {
		fmt.Fprintln(logs, "usage: protocarry check -config CONFIG.json -out NEW_DIR\n       protocarry replay -case CASE_DIR -out NEW_DIR\n       protocarry version")
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(logs)
	evidence := fs.String("out", "", "new evidence directory")
	var source *string
	var adapter, workingDir *string
	if args[0] == "check" {
		source = fs.String("config", "", "JSON config path")
	} else {
		source = fs.String("case", "", "saved case directory (contains case.json)")
		adapter = fs.String("adapter", "", "explicit replacement argv as a JSON array")
		workingDir = fs.String("working-dir", "", "explicit replacement working directory")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *source == "" || *evidence == "" || fs.NArg() != 0 {
		fmt.Fprintln(logs, "required: input flag and -out NEW_DIR")
		return 2
	}
	var report *carry.Report
	var err error
	if args[0] == "check" {
		var c carry.Config
		c, err = carry.LoadConfig(*source)
		if err != nil {
			fmt.Fprintln(logs, "configuration:", err)
			return 2
		}
		report, err = carry.Run(c, *evidence)
	} else {
		opts := carry.ReplayOptions{WorkingDir: *workingDir}
		if *adapter != "" {
			if err = json.Unmarshal([]byte(*adapter), &opts.Adapter); err != nil || len(opts.Adapter) == 0 {
				fmt.Fprintln(logs, "configuration: -adapter must be a nonempty JSON argv array")
				return 2
			}
		}
		report, err = carry.ReplayWithOptions(*source, *evidence, opts)
	}
	if err != nil {
		fmt.Fprintln(logs, err)
		var ce *carry.ConfigError
		if errors.As(err, &ce) {
			return 2
		}
		return 4
	}
	// Nothing resembling a final outcome is printed until evidence has been saved.
	for _, c := range report.Cases {
		fmt.Fprintf(out, "%s %s", c.Outcome, c.ID)
		if c.Reason != "" {
			fmt.Fprintf(out, ": %s", c.Reason)
		}
		fmt.Fprintln(out)
		for _, a := range c.Assertions {
			if a.Preserved != nil && !*a.Preserved {
				fmt.Fprintf(out, "  field %s (%s)\n    expected: %s\n    actual:   %s\n", a.Path, fieldNumbers(a.Numbers), describe(a.Expected), describe(*a.Actual))
				if a.Added {
					fmt.Fprintln(out, "    older schema: field/path absent")
				}
			}
		}
	}
	fmt.Fprintf(out, "%s: planned=%d attempted=%d executed=%d PASS=%d FAIL=%d UNRESOLVED=%d INFRASTRUCTURE_ERROR=%d incomplete=%t\nEvidence: %s/report.json\n", report.Outcome, report.Planned, report.Attempted, report.Executed, report.Counts[carry.Pass], report.Counts[carry.Fail], report.Counts[carry.Unresolved], report.Counts[carry.InfrastructureError], report.Incomplete, *evidence)
	return carry.ExitCode(report.Outcome)
}
func fieldNumbers(numbers []int32) string {
	out := make([]string, len(numbers))
	for i, n := range numbers {
		out[i] = fmt.Sprintf("#%d", n)
	}
	return strings.Join(out, "/")
}
func valueText(v carry.Value) string {
	text := fmt.Sprintf("%q (%s)", v.Value, v.Kind)
	if v.Kind == "bytes" {
		text = fmt.Sprintf("%q (bytes, base64)", v.Value)
	}
	if v.Kind == "message" {
		parts := []string{}
		for _, f := range v.Fields {
			parts = append(parts, fmt.Sprintf("%s (#%d): %s", f.Name, f.Number, valueText(f.Value)))
		}
		text = "{" + strings.Join(parts, ", ") + "}"
	}
	if v.Presence != nil {
		text += fmt.Sprintf(" presence=%t", *v.Presence)
	}
	return text
}
func describe(o carry.Observation) string {
	text := valueText(o.Value)
	if len(o.Parents) > 0 {
		text += fmt.Sprintf(" parent_presence=%v", o.Parents)
	}
	return text
}
