// Command c5egprov-show prints what the c5egprov provider would return, with
// no OpenTofu involved.
//
// It uses the same model package the provider uses, so a parsing problem
// shows up here exactly as it would in a plan - and, unlike a plan, it can be
// stepped through in a debugger:
//
//	dlv debug ./cmd/c5egprov-show -- -data-dir ../data
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/craig5/private-data-provider-example/provider/internal/model"
)

func main() {
	dataDir := flag.String("data-dir", "data", "directory holding roles.yaml and teams/")
	emailDomain := flag.String("email-domain", "example.com", "domain for users with no explicit email")
	format := flag.String("format", "table", "output format: table or json")
	flag.Parse()

	// Everything is a flag. A stray argument is most likely a data directory
	// given without -data-dir, which would otherwise be silently ignored.
	if flag.NArg() > 0 {
		fail(fmt.Errorf("unexpected argument %q (use -data-dir)", flag.Arg(0)))
	}
	write, ok := map[string]func(io.Writer, *model.Directory) error{
		"table": writeTable,
		"json":  writeJSON,
	}[*format]
	if !ok {
		fail(fmt.Errorf("unknown -format %q: want table or json", *format))
	}

	dir, err := model.Load(*dataDir, *emailDomain)
	if err != nil {
		fail(err)
	}
	if err := write(os.Stdout, dir); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// writeJSON prints the data exactly as the provider hands it to OpenTofu.
func writeJSON(w io.Writer, dir *model.Directory) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(dir)
}

// writeTable prints one table each for users, roles and bindings.
func writeTable(w io.Writer, dir *model.Directory) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "USERNAME\tFULL NAME\tTEAM\tEMAIL\tROLES")
	for _, k := range slices.Sorted(maps.Keys(dir.Users)) {
		u := dir.Users[k]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", u.Username, u.FullName, u.Team, u.Email, strings.Join(u.Roles, ", "))
	}

	fmt.Fprintln(tw, "\nROLE\tDESCRIPTION")
	for _, k := range slices.Sorted(maps.Keys(dir.Roles)) {
		r := dir.Roles[k]
		fmt.Fprintf(tw, "%s\t%s\n", r.Name, r.Description)
	}

	fmt.Fprintln(tw, "\nUSER\tROLE")
	for _, k := range slices.Sorted(maps.Keys(dir.Bindings)) {
		b := dir.Bindings[k]
		fmt.Fprintf(tw, "%s\t%s\n", b.User, b.Role)
	}

	return tw.Flush()
}
