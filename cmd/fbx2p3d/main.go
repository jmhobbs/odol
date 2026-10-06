// Command fbx2p3d builds an MLOD P3D from one binary FBX file per
// graphical resolution LOD, adding a "camoground" named selection and the
// given texture/material to each, plus Geometry, View Geometry, Fire
// Geometry, and Memory LODs. See plan_fbx2p3d.md for design rationale.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmhobbs/odol/internal/fbximport"
	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/internal/model"
)

func main() {
	var lods lodFlagList
	flag.Var(&lods, "lod", "graphical LOD as <resolution>=<path.fbx>; repeatable, at least one required")
	texture := flag.String("texture", "", "texture .paa path applied to every graphical LOD (required)")
	material := flag.String("material", "", "material .rvmat path applied to every graphical LOD (required)")
	mass := flag.Float64("mass", 1.0, "total Geometry LOD mass in kg, split evenly across its 8 points")
	scale := flag.Float64("scale", 1.0, "uniform correction factor applied to every imported LOD's vertex positions, to fix a real-world unit mismatch from the source FBX")
	output := flag.String("o", "", "output P3D path (default: derived from the lowest-resolution --lod file)")
	var yes bool
	flag.BoolVar(&yes, "y", false, "skip the size-preview confirmation prompt (also auto-skipped when stdin is not a terminal)")
	flag.BoolVar(&yes, "yes", false, "long form of -y")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s --lod <resolution>=<path.fbx> [--lod ...] --texture <path.paa> --material <path.rvmat> [--scale <factor>] [-y] [-o output.p3d]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(lods) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one --lod is required")
		flag.Usage()
		os.Exit(2)
	}
	if *texture == "" || *material == "" {
		fmt.Fprintln(os.Stderr, "error: --texture and --material are required")
		flag.Usage()
		os.Exit(2)
	}
	if err := validateScale(*scale); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}

	opts := mlodbuild.Options{
		Texture:  *texture,
		Material: *material,
		Mass:     float32(*mass),
	}
	outPath := outputPath(lods, *output)

	skipPrompt := yes || !isInteractive(os.Stdin)
	confirm := newConfirmFunc(os.Stderr, os.Stdin, skipPrompt)

	if err := run(lods, opts, float32(*scale), outPath, os.Stderr, confirm); err != nil {
		if errors.Is(err, errCancelled) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// validateScale rejects scale factors that can't produce sensible geometry:
// zero collapses every vertex to a single point, and negative values invert
// winding/normals in ways nothing downstream handles.
func validateScale(scale float64) error {
	if scale <= 0 {
		return fmt.Errorf("--scale must be greater than 0, got %g", scale)
	}
	return nil
}

// scaleVertices returns a new slice with every vertex multiplied by factor.
// It does not mutate vertices.
func scaleVertices(vertices []model.Vector3, factor float32) []model.Vector3 {
	out := make([]model.Vector3, len(vertices))
	for i, v := range vertices {
		out[i] = model.Vector3{X: v.X * factor, Y: v.Y * factor, Z: v.Z * factor}
	}
	return out
}

// lodEntry is one parsed --lod flag value.
type lodEntry struct {
	Resolution float32
	Path       string
}

// lodFlagList implements flag.Value so --lod can be repeated.
type lodFlagList []lodEntry

func (l *lodFlagList) String() string {
	if l == nil {
		return ""
	}
	parts := make([]string, len(*l))
	for i, e := range *l {
		parts[i] = fmt.Sprintf("%g=%s", e.Resolution, e.Path)
	}
	return strings.Join(parts, ",")
}

func (l *lodFlagList) Set(s string) error {
	e, err := parseLODFlag(s)
	if err != nil {
		return err
	}
	*l = append(*l, e)
	return nil
}

// parseLODFlag parses one --lod value in "<resolution>=<path.fbx>" form.
func parseLODFlag(s string) (lodEntry, error) {
	idx := strings.IndexByte(s, '=')
	if idx < 0 {
		return lodEntry{}, fmt.Errorf("invalid --lod value %q: expected <resolution>=<path.fbx>", s)
	}
	resStr, path := s[:idx], s[idx+1:]
	if path == "" {
		return lodEntry{}, fmt.Errorf("invalid --lod value %q: missing FBX path", s)
	}
	res, err := strconv.ParseFloat(resStr, 32)
	if err != nil {
		return lodEntry{}, fmt.Errorf("invalid --lod value %q: bad resolution %q: %w", s, resStr, err)
	}
	return lodEntry{Resolution: float32(res), Path: path}, nil
}

// outputPath returns explicit if set, otherwise a path derived from the
// lowest-resolution (most detailed) --lod file's basename.
func outputPath(lods []lodEntry, explicit string) string {
	if explicit != "" {
		return explicit
	}
	best := lods[0]
	for _, e := range lods[1:] {
		if e.Resolution < best.Resolution {
			best = e
		}
	}
	dir := filepath.Dir(best.Path)
	base := strings.TrimSuffix(filepath.Base(best.Path), filepath.Ext(best.Path))
	return filepath.Join(dir, base+".p3d")
}

func run(lods []lodEntry, opts mlodbuild.Options, scale float32, outPath string, logOutput io.Writer, confirm confirmFunc) error {
	logger := stepLogger{w: logOutput}

	var graphical []mlodbuild.GraphicalLOD
	for i, e := range lods {
		logger.logf("importing LOD %g from %s", e.Resolution, e.Path)
		data, err := os.ReadFile(e.Path)
		if err != nil {
			return err
		}
		mesh, err := fbximport.Import(data)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Path, err)
		}
		if mesh.BestEffortPolygons > 0 {
			logger.logf("warning: %s has %d self-intersecting or zero-area polygon(s), imported best-effort (triangles may be flipped or overlap). Fix them in the source model.", e.Path, mesh.BestEffortPolygons)
		}
		scaledVertices := scaleVertices(mesh.Vertices, scale)

		if i == 0 && confirm != nil {
			if err := confirm(e.Path, boundingBoxSize(scaledVertices)); err != nil {
				return err
			}
		}

		graphical = append(graphical, mlodbuild.GraphicalLOD{
			Resolution: e.Resolution,
			Vertices:   scaledVertices,
			Normals:    mesh.Normals,
			Faces:      mesh.Faces,
		})
	}

	logger.logf("building auxiliary LODs (Geometry, View Geometry, Fire Geometry, Memory)")
	m, err := mlodbuild.Build(graphical, opts)
	if err != nil {
		return err
	}

	data, err := mlod.Marshal(m)
	if err != nil {
		return err
	}

	logger.logf("writing %s", outPath)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
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
	_, _ = fmt.Fprintf(l.w, "fbx2p3d: "+format+"\n", args...)
}
