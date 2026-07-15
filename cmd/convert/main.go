package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmhobbs/odol/internal/convert"
)

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
	base := baseName(inputPath)
	return filepath.Join(dir, base+"_mlod.p3d"), filepath.Join(dir, base+".model.cfg")
}

func fbxOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := baseName(inputPath)
	return filepath.Join(dir, base+".fbx")
}

func baseName(inputPath string) string {
	return strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
}

func runWithOutputsLogged(inputPath, outputP3DPath, outputConfigPath string, logOutput io.Writer) error {
	logger := stepLogger{w: logOutput}
	logger.logf("       input P3D: %s", inputPath)
	logger.logf("     output MLOD: %s", outputP3DPath)
	logger.logf("output model.cfg: %s\n", outputConfigPath)

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	p3d, modelCfg, err := convert.ConvertToMLOD(data, baseName(inputPath), logger.logf)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputP3DPath, p3d, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(outputConfigPath, []byte(modelCfg), 0o644); err != nil {
		return err
	}

	logger.logf("conversion complete")
	return nil
}

func runFBXWithOutputLogged(inputPath, outputFBXPath string, ascii bool, logOutput io.Writer) error {
	logger := stepLogger{w: logOutput}
	logger.logf("  input P3D: %s", inputPath)
	logger.logf(" output FBX: %s", outputFBXPath)

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	fbx, err := convert.ConvertToFBX(data, baseName(inputPath), ascii, logger.logf)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputFBXPath, fbx, 0o644); err != nil {
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
