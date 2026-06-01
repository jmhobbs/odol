package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/model"
	"github.com/jmhobbs/odol/internal/modelcfg"
	"github.com/jmhobbs/odol/internal/odol"
)

type partialParseError struct {
	LODIndex    int
	Resolution  float32
	PropertyKey string
}

func (e partialParseError) Error() string {
	return fmt.Sprintf(
		"input contains partially decoded LOD %d (resolution %g, marker %q); refusing to emit lossy MLOD",
		e.LODIndex,
		e.Resolution,
		e.PropertyKey,
	)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <input.p3d>\n", os.Args[0])
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inputPath string, logOutput io.Writer) error {
	outputP3DPath, outputConfigPath := outputPaths(inputPath)
	return runWithOutputsLogged(inputPath, outputP3DPath, outputConfigPath, logOutput)
}

func outputPaths(inputPath string) (string, string) {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := filepath.Base(inputPath)
	if ext != "" {
		base = base[:len(base)-len(ext)]
	}

	return filepath.Join(dir, base+"_mlod.p3d"), filepath.Join(dir, base+".model.cfg")
}

func runWithOutputsLogged(inputPath, outputP3DPath, outputConfigPath string, logOutput io.Writer) error {
	logger := stepLogger{w: logOutput}
	logger.logf("       input P3D: %s", inputPath)
	logger.logf("     output MLOD: %s", outputP3DPath)
	logger.logf("output model.cfg: %s\n", outputConfigPath)

	format, err := detector.DetectFile(inputPath)
	if err != nil {
		return err
	}
	logger.logf("detected input format: %s", format.String())
	if format.Family != detector.P3DFamilyODOL {
		return fmt.Errorf("unsupported input format: %s", format.String())
	}
	logger.logf("converting %s to MLOD + model.cfg", format.String())

	logger.logf("parsing ODOL data")
	parsed, err := odol.ParseFileStrict(inputPath)
	if err != nil {
		return err
	}
	logger.logf("validating full decode")
	if err := ensureFullyDecoded(parsed); err != nil {
		return err
	}

	logger.logf("writing output files")
	if err := mlod.WriteFile(outputP3DPath, parsed); err != nil {
		return err
	}

	renderedConfig, err := modelcfg.Render(parsed.Config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputConfigPath, []byte(renderedConfig), 0o644); err != nil {
		return err
	}

	logger.logf("conversion complete")
	return nil
}

type stepLogger struct {
	w io.Writer
}

func (l stepLogger) logf(format string, args ...any) {
	if l.w == nil {
		return
	}
	_, _ = fmt.Fprintf(l.w, "convert: "+format+"\n", args...)
}

func ensureFullyDecoded(parsed *model.Model) error {
	for i, lod := range parsed.LODs {
		if _, ok := lod.Properties["odol_partial_parse"]; ok {
			return partialParseError{
				LODIndex:    i,
				Resolution:  lod.Resolution,
				PropertyKey: "odol_partial_parse",
			}
		}
	}

	return nil
}
