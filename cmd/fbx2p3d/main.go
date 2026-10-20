// Command fbx2p3d builds an MLOD P3D from one binary FBX file per
// graphical resolution LOD, adding a "camoground" named selection and the
// given texture/material (empty when not given) to each, plus Geometry,
// View Geometry, Fire Geometry, and Memory LODs. See plan_fbx2p3d.md for
// design rationale.
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

	"github.com/jmhobbs/odol/internal/fbx2p3d"
	"github.com/jmhobbs/odol/internal/fbximport"
	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/mlodbuild"
)

func main() {
	var lods lodFlagList
	flag.Var(&lods, "lod", "graphical LOD as <resolution>=<path.fbx>; repeatable, at least one required")
	texture := flag.String("texture", "", "texture .paa path applied to every graphical LOD (default: none)")
	material := flag.String("material", "", "material .rvmat path applied to every graphical LOD (default: none)")
	fireMaterial := flag.String("fire-material", "", "vanilla penetration material applied to the Fire Geometry LOD, by name (e.g. wood, metalplate) or full DZ\\data\\data\\penetration path; valid names are listed below")
	mass := flag.Float64("mass", 1.0, "total Geometry LOD mass in kg, split evenly across its 8 points")
	scale := flag.Float64("scale", 1.0, "uniform correction factor applied to every imported LOD's vertex positions, to fix a real-world unit mismatch from the source FBX")
	output := flag.String("o", "", "output P3D path (default: derived from the lowest-resolution --lod file)")
	var normals fbximport.NormalConvention
	flag.Var(&normals, "normals", "how FBX normals convert: auto (detect per mesh), dayz (DayZ tools, Z mirrored), or standard (Blender and most other tools)")
	var yes bool
	flag.BoolVar(&yes, "y", false, "skip the size-preview confirmation prompt (also auto-skipped when stdin is not a terminal)")
	flag.BoolVar(&yes, "yes", false, "long form of -y")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s --lod <resolution>=<path.fbx> [--lod ...] [--texture <path.paa>] [--material <path.rvmat>] [--fire-material <name>] [--scale <factor>] [--normals auto|dayz|standard] [-y] [-o output.p3d]\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nvalid --fire-material names:")
		fmt.Fprint(os.Stderr, wrapList(mlodbuild.PenetrationMaterialNames(), "  ", 80))
	}
	flag.Parse()

	if len(lods) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one --lod is required")
		flag.Usage()
		os.Exit(2)
	}
	if err := fbx2p3d.ValidateScale(*scale); err != nil {
		fmt.Fprintf(os.Stderr, "error: --%v\n", err)
		flag.Usage()
		os.Exit(2)
	}
	var fireMaterialPath string
	if *fireMaterial != "" {
		var err error
		fireMaterialPath, err = mlodbuild.PenetrationMaterialPath(*fireMaterial)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: --fire-material:", err)
			os.Exit(2)
		}
	}

	opts := mlodbuild.Options{
		Texture:              *texture,
		Material:             *material,
		FireGeometryMaterial: fireMaterialPath,
		Mass:                 float32(*mass),
	}
	outPath := outputPath(lods, *output)

	skipPrompt := yes || !isInteractive(os.Stdin)
	confirm := newConfirmFunc(os.Stderr, os.Stdin, skipPrompt)

	if err := run(lods, opts, float32(*scale), normals, outPath, os.Stderr, confirm); err != nil {
		if errors.Is(err, errCancelled) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// wrapList joins items with ", " into newline-terminated lines that start
// with indent and stay within maxWidth. An item too long to fit gets a line
// of its own rather than being split.
func wrapList(items []string, indent string, maxWidth int) string {
	var out strings.Builder
	line := indent
	for i, item := range items {
		if i < len(items)-1 {
			item += ","
		}
		if line != indent && len(line)+1+len(item) > maxWidth {
			out.WriteString(line + "\n")
			line = indent
		}
		if line != indent {
			line += " "
		}
		line += item
	}
	if line != indent {
		out.WriteString(line + "\n")
	}
	return out.String()
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

func run(lods []lodEntry, opts mlodbuild.Options, scale float32, normals fbximport.NormalConvention, outPath string, logOutput io.Writer, confirm confirmFunc) error {
	logger := stepLogger{w: logOutput}

	sources := make([]fbx2p3d.LODSource, len(lods))
	for i, e := range lods {
		sources[i] = fbx2p3d.LODSource{
			Resolution: e.Resolution,
			Name:       e.Path,
			Load:       func() ([]byte, error) { return os.ReadFile(e.Path) },
			Normals:    normals,
		}
	}
	hooks := fbx2p3d.Hooks{
		Logf:          logger.logf,
		Warnf:         func(format string, args ...any) { logger.logf("warning: "+format, args...) },
		AfterFirstLOD: confirm,
	}

	m, err := fbx2p3d.Build(sources, opts, scale, hooks)
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
