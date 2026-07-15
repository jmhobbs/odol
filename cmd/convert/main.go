package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/fbxexport"
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
	fbx := flag.Bool("fbx", false, "export FBX instead of MLOD")
	fbxASCII := flag.Bool("fbx-ascii", false, "export ASCII FBX instead of binary (implies --fbx)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--fbx] [--fbx-ascii] <input.p3d>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(2)
	}

	useFBX, useASCII := resolveFBXMode(*fbx, *fbxASCII)

	var err error
	if useFBX {
		err = runFBX(args[0], useASCII, os.Stderr)
	} else {
		err = run(args[0], os.Stderr)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// resolveFBXMode applies the "--fbx-ascii implies --fbx" rule.
func resolveFBXMode(fbx, fbxASCII bool) (useFBX, useASCII bool) {
	return fbx || fbxASCII, fbxASCII
}

func run(inputPath string, logOutput io.Writer) error {
	outputP3DPath, outputConfigPath := outputPaths(inputPath)
	return runWithOutputsLogged(inputPath, outputP3DPath, outputConfigPath, logOutput)
}

func runFBX(inputPath string, ascii bool, logOutput io.Writer) error {
	outputFBXPath := fbxOutputPath(inputPath)
	return runFBXWithOutputLogged(inputPath, outputFBXPath, ascii, logOutput)
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

func fbxOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := filepath.Base(inputPath)
	if ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return filepath.Join(dir, base+".fbx")
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

func runFBXWithOutputLogged(inputPath, outputFBXPath string, ascii bool, logOutput io.Writer) error {
	logger := stepLogger{w: logOutput}
	logger.logf("  input P3D: %s", inputPath)
	logger.logf(" output FBX: %s", outputFBXPath)

	format, err := detector.DetectFile(inputPath)
	if err != nil {
		return err
	}
	logger.logf("detected input format: %s", format.String())

	var parsed *model.Model
	switch format.Family {
	case detector.P3DFamilyODOL:
		logger.logf("converting %s to FBX", format.String())
		logger.logf("parsing ODOL data")
		parsed, err = odol.ParseFileStrict(inputPath)
		if err != nil {
			return err
		}
		logger.logf("validating full decode")
		if err = ensureFullyDecoded(parsed); err != nil {
			return err
		}
	case detector.P3DFamilyMLOD:
		logger.logf("converting MLOD/P3DM to FBX")
		logger.logf("parsing MLOD data")
		parsed, err = mlod.ParseFile(inputPath)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported input format for FBX export: %s", format.String())
	}

	logger.logf("writing FBX")
	if ascii {
		if err := fbxexport.WriteASCIIFile(outputFBXPath, parsed); err != nil {
			return err
		}
	} else {
		if err := fbxexport.WriteFile(outputFBXPath, parsed); err != nil {
			return err
		}
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
