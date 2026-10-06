package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/jmhobbs/odol/internal/model"
)

// referenceHumanHeightMeters is the Y-axis (up) extent of LOD 0 (the
// highest-detail visual LOD, resolution 1.0) in
// samples/mlod/character_male.p3d, this project's player rig reference -
// measured via internal/mlod.Parse plus a min/max sweep over its vertices,
// not rounded to the nominal "1.8m" expectation so the constant's
// provenance stays traceable to the actual measurement.
const referenceHumanHeightMeters = 1.7976

// errCancelled is returned by confirmContinue, and therefore by any
// confirmFunc built on it, when the user declines to proceed.
var errCancelled = errors.New("fbx2p3d: cancelled by user")

// boundingBoxSize returns the (width, height, depth) extent of vertices
// along X, Y, Z respectively. MLOD/ODOL use Y as the up axis.
func boundingBoxSize(vertices []model.Vector3) model.Vector3 {
	if len(vertices) == 0 {
		return model.Vector3{}
	}
	min, max := vertices[0], vertices[0]
	for _, v := range vertices[1:] {
		if v.X < min.X {
			min.X = v.X
		}
		if v.Y < min.Y {
			min.Y = v.Y
		}
		if v.Z < min.Z {
			min.Z = v.Z
		}
		if v.X > max.X {
			max.X = v.X
		}
		if v.Y > max.Y {
			max.Y = v.Y
		}
		if v.Z > max.Z {
			max.Z = v.Z
		}
	}
	return model.Vector3{X: max.X - min.X, Y: max.Y - min.Y, Z: max.Z - min.Z}
}

// printSizePreview writes a human-readable bounding box and a
// height-vs-reference comparison to w.
func printSizePreview(w io.Writer, lodPath string, size model.Vector3) {
	ratio := float64(size.Y) / referenceHumanHeightMeters
	_, _ = fmt.Fprintf(w, "fbx2p3d: preview: %s bounding box (X,Y,Z) = %.4fm x %.4fm x %.4fm\n",
		lodPath, size.X, size.Y, size.Z)
	_, _ = fmt.Fprintf(w, "fbx2p3d: preview: height %.4fm is %.2fx the reference human height (%.4fm, measured from samples/mlod/character_male.p3d)\n",
		size.Y, ratio, referenceHumanHeightMeters)
}

// isInteractive reports whether f is a real terminal, as opposed to a pipe,
// redirect, regular file, or /dev/null. A plain os.ModeCharDevice stat
// check is NOT sufficient here: /dev/null is itself a character device, so
// that heuristic misreports "--lod ... </dev/null" (the exact
// scripted/non-interactive case this exists to handle) as interactive.
// term.IsTerminal performs the real isatty ioctl instead.
func isInteractive(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// confirmContinue prints a y/N prompt to w and reads one answer line from
// r. Only "y" or "yes" (case-insensitive) count as an affirmative; anything
// else - including an empty line or EOF (e.g. closed/non-interactive
// stdin) - fails safe by returning errCancelled rather than hanging or
// silently proceeding.
func confirmContinue(r io.Reader, w io.Writer) error {
	_, _ = fmt.Fprint(w, "fbx2p3d: continue with this scale? [y/N] ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return errCancelled
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if answer == "y" || answer == "yes" {
		return nil
	}
	return errCancelled
}

// confirmFunc is the seam run() depends on for its post-first-LOD size
// check, instead of talking to os.Stdin/flags directly - keeps run()
// trivially testable with a stub.
type confirmFunc func(lodPath string, size model.Vector3) error

// newConfirmFunc builds the confirmFunc main() wires into run(): it always
// prints the size preview to w, and additionally blocks on a y/N prompt
// read from stdin unless skipPrompt is true.
func newConfirmFunc(w io.Writer, stdin io.Reader, skipPrompt bool) confirmFunc {
	return func(lodPath string, size model.Vector3) error {
		printSizePreview(w, lodPath, size)
		if skipPrompt {
			return nil
		}
		return confirmContinue(stdin, w)
	}
}
