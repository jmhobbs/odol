// Package fbx2p3d holds the FBX-to-MLOD build steps shared by cmd/fbx2p3d
// (CLI) and cmd/wasm (browser): import each graphical LOD's FBX, apply the
// scale factor, and assemble the full model with internal/mlodbuild. Like
// internal/convert, it does no file I/O of its own - callers supply each
// LOD's bytes through LODSource.Load. See plans/plan_fbx2p3d-web.md.
package fbx2p3d

import (
	"fmt"

	"github.com/jmhobbs/odol/internal/fbximport"
	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/internal/model"
)

// ReferenceHumanHeightMeters is the Y-axis (up) extent of LOD 0 (the
// highest-detail visual LOD, resolution 1.0) in
// samples/mlod/character_male.p3d, this project's player rig reference -
// measured via internal/mlod.Parse plus a min/max sweep over its vertices,
// not rounded to the nominal "1.8m" expectation so the constant's
// provenance stays traceable to the actual measurement.
const ReferenceHumanHeightMeters = 1.7976

// LODSource is one graphical LOD to import from an FBX file.
type LODSource struct {
	Resolution float32
	// Name identifies the source in log lines and errors: the --lod path in
	// the CLI, the picked file's name in the browser.
	Name string
	// Load returns the FBX bytes. Build calls it only when it reaches this
	// LOD, so a declined AfterFirstLOD hook never reads later files.
	Load func() ([]byte, error)
}

// Hooks lets callers observe and gate Build. Every field is optional.
type Hooks struct {
	// Logf receives one progress line per step.
	Logf func(format string, args ...any)
	// Warnf receives problems that don't stop the build, such as polygons
	// imported best-effort.
	Warnf func(format string, args ...any)
	// AfterFirstLOD runs once, after LOD 0 is imported and scaled and before
	// any other LOD is loaded, with LOD 0's bounding box size. A non-nil
	// error stops Build and is returned unchanged.
	AfterFirstLOD func(name string, size model.Vector3) error
}

func (h Hooks) logf(format string, args ...any) {
	if h.Logf != nil {
		h.Logf(format, args...)
	}
}

func (h Hooks) warnf(format string, args ...any) {
	if h.Warnf != nil {
		h.Warnf(format, args...)
	}
}

// Build imports every source in order, scales its vertices by scale, and
// assembles the full MLOD model (graphical LODs plus Geometry, View
// Geometry, Fire Geometry and Memory) with mlodbuild.Build.
func Build(sources []LODSource, opts mlodbuild.Options, scale float32, hooks Hooks) (*model.Model, error) {
	var graphical []mlodbuild.GraphicalLOD
	for i, source := range sources {
		hooks.logf("importing LOD %g from %s", source.Resolution, source.Name)
		data, err := source.Load()
		if err != nil {
			return nil, err
		}
		mesh, err := fbximport.Import(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source.Name, err)
		}
		if mesh.BestEffortPolygons > 0 {
			hooks.warnf("%s has %d self-intersecting or zero-area polygon(s), imported best-effort (triangles may be flipped or overlap). Fix them in the source model.", source.Name, mesh.BestEffortPolygons)
		}
		scaledVertices := scaleVertices(mesh.Vertices, scale)

		if i == 0 && hooks.AfterFirstLOD != nil {
			if err := hooks.AfterFirstLOD(source.Name, BoundingBoxSize(scaledVertices)); err != nil {
				return nil, err
			}
		}

		graphical = append(graphical, mlodbuild.GraphicalLOD{
			Resolution: source.Resolution,
			Vertices:   scaledVertices,
			Normals:    mesh.Normals,
			Faces:      mesh.Faces,
		})
	}

	hooks.logf("building auxiliary LODs (Geometry, View Geometry, Fire Geometry, Memory)")
	return mlodbuild.Build(graphical, opts)
}

// ValidateScale rejects scale factors that can't produce sensible geometry:
// zero collapses every vertex to a single point, and negative values invert
// winding/normals in ways nothing downstream handles.
func ValidateScale(scale float64) error {
	if scale <= 0 {
		return fmt.Errorf("scale must be greater than 0, got %g", scale)
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

// BoundingBoxSize returns the (width, height, depth) extent of vertices
// along X, Y, Z respectively. MLOD/ODOL use Y as the up axis.
func BoundingBoxSize(vertices []model.Vector3) model.Vector3 {
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
