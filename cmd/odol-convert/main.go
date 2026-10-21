package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmhobbs/odol/internal/convert"
)

const pausePrompt = "Press Enter to close this window...\n"

type cliOptions struct {
	stdin  io.Reader
	stderr io.Writer
	// pauseOnExit waits for Enter before returning, so a console window
	// opened by a drag and drop in Explorer stays open long enough to read.
	pauseOnExit bool
}

func main() {
	os.Exit(runCLI(os.Args[1:], cliOptions{
		stdin:       os.Stdin,
		stderr:      os.Stderr,
		pauseOnExit: consoleClosesOnExit(),
	}))
}

func runCLI(args []string, options cliOptions) int {
	if options.pauseOnExit {
		defer waitForEnter(options.stdin, options.stderr)
	}

	flags := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	flags.SetOutput(options.stderr)
	fbx := flags.Bool("fbx", false, "export FBX instead of MLOD")
	fbxASCII := flags.Bool("fbx-ascii", false, "export ASCII FBX instead of binary (implies --fbx)")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(options.stderr, "usage: %s [--fbx] [--fbx-ascii] <input.p3d> [<input.p3d> ...]\n", flags.Name())
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}

	inputPaths := flags.Args()
	if len(inputPaths) == 0 {
		flags.Usage()
		return 2
	}

	useFBX, useASCII := resolveFBXMode(*fbx, *fbxASCII)

	exitCode := 0
	for _, inputPath := range inputPaths {
		var err error
		if useFBX {
			err = runFBX(inputPath, useASCII, options.stderr)
		} else {
			err = run(inputPath, options.stderr)
		}
		if err != nil {
			_, _ = fmt.Fprintf(options.stderr, "convert: %s: %v\n", inputPath, err)
			exitCode = 1
		}
	}
	return exitCode
}

func waitForEnter(stdin io.Reader, stderr io.Writer) {
	_, _ = fmt.Fprint(stderr, pausePrompt)
	_, _ = bufio.NewReader(stdin).ReadString('\n')
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
