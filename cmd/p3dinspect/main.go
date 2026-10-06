// Command p3dinspect summarizes a P3D model (MLOD or ODOL): LODs, point/
// face counts, textures, materials, and named selections.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/inspect"
	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/model"
	"github.com/jmhobbs/odol/internal/odol"
)

func main() {
	useJSON := flag.Bool("json", false, "output the summary as JSON instead of text")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--json] <input.p3d>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(args[0], os.Stdout, *useJSON); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string, stdout io.Writer, useJSON bool) error {
	format, err := detector.DetectFile(path)
	if err != nil {
		return err
	}

	parsed, err := parseModel(path, format)
	if err != nil {
		return err
	}

	summary := inspect.Summarize(parsed, format)

	if useJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	return inspect.WriteText(stdout, summary)
}

func parseModel(path string, format detector.P3DFormat) (*model.Model, error) {
	switch format.Family {
	case detector.P3DFamilyMLOD:
		return mlod.ParseFile(path)
	case detector.P3DFamilyODOL:
		return odol.ParseFile(path)
	default:
		return nil, fmt.Errorf("unsupported format for inspection: %s", format.String())
	}
}
