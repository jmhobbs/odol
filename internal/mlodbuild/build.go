// Package mlodbuild assembles a full MLOD model.Model out of one or more
// imported graphical LODs plus a texture/material/mass configuration: it
// adds the "camoground" named selection and per-face texture/material to
// each graphical LOD, and generates the four fixed auxiliary LODs (Geometry,
// View Geometry, Fire Geometry, Memory) required by cmd/fbx2p3d. See
// plan_fbx2p3d.md for the design decisions behind this package.
package mlodbuild

import (
	"fmt"
	"math"

	"github.com/jmhobbs/odol/internal/model"
)

// Special LOD resolution floats, from docs/P3DModelInfo.txt's documented
// table. Values below 1000 are graphical LODs; these four are the fixed
// points the game engine matches by exact resolution value, not distance.
const (
	GeometryResolution     float32 = 1.0e13
	MemoryResolution       float32 = 1.0e15
	ViewGeometryResolution float32 = 6.0e15
	FireGeometryResolution float32 = 7.0e15
)

// maxGraphicalResolution is the documented upper bound for a graphical
// (visual) LOD - docs/P3DModelInfo.txt: "If the value equals or is above
// 10000.0 ... it is a shadow LOD" (functional/special LODs use even larger,
// specific values). Values from 1000 up to this bound are also reserved
// (view/shadow distances), so graphical LODs supplied to Build must stay
// below 1000.
const maxGraphicalResolution = 1000.0

// CamogroundSelectionName is the named selection written on every graphical
// LOD, covering all of its geometry.
const CamogroundSelectionName = "camoground"

// Component01SelectionName is the named selection written on every
// collision LOD (Geometry, View Geometry, Fire Geometry), covering all of
// its geometry. Arma's convex-hull physics requires each convex piece of a
// collision LOD to be marked via a "ComponentNN" named selection; since
// these LODs are always a single convex box (see boxGeometry), one such
// selection covering the whole box is correct and sufficient.
const Component01SelectionName = "Component01"

// Memory LOD point names. All five are added automatically by Build; none
// are configurable, so a P3D's Memory LOD always has exactly this shape.
const (
	// MemoryCenterName is the LOD's origin-point selection.
	MemoryCenterName = "ce_center"
	// InvViewName is the inventory-icon camera point: centered in front of
	// the object (local +Z, the RV engine's forward axis at 0 azimuth) and
	// vertically centered on the bounding box, far enough out to keep the
	// whole object in frame - see memoryLOD.
	InvViewName = "invview"
	// BoundingBoxMinName/BoundingBoxMaxName are the collision box's own
	// min/max corners, matching the vanilla naming convention used by
	// deployables' hologram-placement fallback (see
	// docs/reference/dayz-p3d-audit/SKILL.md, "box_placing_min/max").
	BoundingBoxMinName = "boundingbox_min"
	BoundingBoxMaxName = "boundingbox_max"
	// CentralEconomyRadiusName is a point placed at (radius, 0, radius) on
	// (X, 0, Z), where radius is half the object's largest horizontal
	// (X/Z) extent - a ground-footprint radius for central-economy
	// spawn/placement exclusion.
	CentralEconomyRadiusName = "ce_radius"
)

// GraphicalLOD is one visual-resolution LOD's decoded mesh, as produced by
// internal/fbximport, tagged with the resolution it should be written at.
type GraphicalLOD struct {
	Resolution float32
	Vertices   []model.Vector3
	Normals    []model.Vector3
	Faces      []model.Face
}

// Options configures Build.
type Options struct {
	Texture  string  // .paa path written to every graphical LOD's faces
	Material string  // .rvmat path written to every graphical LOD's faces
	Mass     float32 // total mass (kg), split evenly across Geometry LOD's 8 points
}

// Build assembles a complete MLOD model.Model: the supplied graphical LODs
// (each gets a "camoground" selection and the configured texture/material),
// plus Geometry/View Geometry/Fire Geometry LODs (a shared bounding-box hull
// around the union of all graphical LODs' vertices) and a five-point Memory
// LOD (see the Memory LOD point name constants and memoryLOD).
func Build(graphical []GraphicalLOD, opts Options) (*model.Model, error) {
	if len(graphical) == 0 {
		return nil, fmt.Errorf("mlodbuild: at least one graphical LOD is required")
	}

	m := &model.Model{}
	seenResolution := make(map[float32]bool, len(graphical))
	var allVertices []model.Vector3
	for _, g := range graphical {
		if g.Resolution >= maxGraphicalResolution {
			return nil, fmt.Errorf("mlodbuild: graphical LOD resolution %g must be below %g", g.Resolution, maxGraphicalResolution)
		}
		if seenResolution[g.Resolution] {
			return nil, fmt.Errorf("mlodbuild: duplicate graphical LOD resolution %g", g.Resolution)
		}
		seenResolution[g.Resolution] = true
		allVertices = append(allVertices, g.Vertices...)

		lod := model.LOD{
			Resolution: g.Resolution,
			Vertices:   g.Vertices,
			Normals:    g.Normals,
			Faces:      withTextureMaterial(g.Faces, opts.Texture, opts.Material),
			Selections: []model.Selection{fullSelection(CamogroundSelectionName, len(g.Vertices), len(g.Faces))},
		}
		m.LODs = append(m.LODs, lod)
	}

	// Recenter X/Z and floor Y using one offset derived from the union of
	// every graphical LOD's vertices, so all LODs (and the aux hull below,
	// which reuses the same translated set) end up consistently positioned
	// relative to each other rather than each independently centered on
	// itself. Unconditional by design - see plan_fbx2p3d-recenter.md.
	offset := centeringOffset(allVertices)
	for i := range m.LODs {
		m.LODs[i].Vertices = translateVertices(m.LODs[i].Vertices, offset)
	}
	allVertices = translateVertices(allVertices, offset)

	box := boundingBox(allVertices)
	boxNormals, boxFaces := boxGeometry(box)
	boxComponentSelection := []model.Selection{fullSelection(Component01SelectionName, len(box), len(boxFaces))}

	m.LODs = append(m.LODs,
		model.LOD{
			Resolution:  GeometryResolution,
			Vertices:    box,
			Normals:     boxNormals,
			Faces:       boxFaces,
			PointMasses: evenMasses(opts.Mass, len(box)),
			Selections:  boxComponentSelection,
		},
		model.LOD{
			Resolution: ViewGeometryResolution,
			Vertices:   box,
			Normals:    boxNormals,
			Faces:      boxFaces,
			Selections: boxComponentSelection,
		},
		model.LOD{
			Resolution: FireGeometryResolution,
			Vertices:   box,
			Normals:    boxNormals,
			Faces:      boxFaces,
			Selections: boxComponentSelection,
		},
		memoryLOD(box),
	)

	return m, nil
}

// withTextureMaterial returns a copy of faces with Texture/Material set on
// every one, leaving all other fields untouched.
func withTextureMaterial(faces []model.Face, texture, material string) []model.Face {
	out := make([]model.Face, len(faces))
	for i, f := range faces {
		f.Texture = texture
		f.Material = material
		out[i] = f
	}
	return out
}

// fullSelection builds a named selection covering every point and face of a
// LOD with numVertices points and numFaces faces.
func fullSelection(name string, numVertices, numFaces int) model.Selection {
	sel := model.Selection{
		Name:          name,
		VertexIndices: make([]uint32, numVertices),
		FaceIndices:   make([]uint32, numFaces),
	}
	for i := range sel.VertexIndices {
		sel.VertexIndices[i] = uint32(i)
	}
	for i := range sel.FaceIndices {
		sel.FaceIndices[i] = uint32(i)
	}
	return sel
}

// evenMasses splits totalMass evenly across n points.
func evenMasses(totalMass float32, n int) []float32 {
	if n == 0 {
		return nil
	}
	out := make([]float32, n)
	each := totalMass / float32(n)
	for i := range out {
		out[i] = each
	}
	return out
}

// memoryLOD builds the fixed five-point Memory LOD, each point identified by
// its own named selection over exactly one vertex (matching the real Arma
// convention - see Model Config.txt's "memory" axis-source references):
//
//   - ce_center: local origin, unconditionally.
//   - boundingbox_min/boundingbox_max: box's own min/max corners. box is in
//     boundingBox's documented corner order, so box[0] is (minX,minY,minZ)
//     and box[6] is (maxX,maxY,maxZ).
//   - invview: horizontally centered (X=0, already true post-recentering)
//     and vertically centered on the box (Y=box height/2, since Y is
//     floored to 0 post-recentering), placed along local +Z (the RV
//     engine's forward axis at 0 azimuth) at 2x the largest bounding-box
//     dimension - generous clearance to keep the whole object in the
//     inventory camera's frame regardless of its aspect ratio or the
//     camera's FOV.
//   - ce_radius: (radius, 0, radius), where radius is half the largest
//     horizontal (X/Z) extent - see CentralEconomyRadiusName.
func memoryLOD(box []model.Vector3) model.LOD {
	boxMin, boxMax := box[0], box[6]
	size := model.Vector3{X: boxMax.X - boxMin.X, Y: boxMax.Y - boxMin.Y, Z: boxMax.Z - boxMin.Z}
	largestDimension := max(size.X, size.Y, size.Z)
	largestHorizontalExtent := max(size.X, size.Z)

	vertices := []model.Vector3{
		{}, // ce_center
		{X: 0, Y: boxMax.Y / 2, Z: largestDimension * 2}, // invview
		boxMin, // boundingbox_min
		boxMax, // boundingbox_max
		{X: largestHorizontalExtent / 2, Y: 0, Z: largestHorizontalExtent / 2}, // ce_radius
	}

	return model.LOD{
		Resolution: MemoryResolution,
		Vertices:   vertices,
		Selections: []model.Selection{
			{Name: MemoryCenterName, VertexIndices: []uint32{0}},
			{Name: InvViewName, VertexIndices: []uint32{1}},
			{Name: BoundingBoxMinName, VertexIndices: []uint32{2}},
			{Name: BoundingBoxMaxName, VertexIndices: []uint32{3}},
			{Name: CentralEconomyRadiusName, VertexIndices: []uint32{4}},
		},
	}
}

// centeringOffset returns the translation that recenters vertices' X/Z at 0
// and floors their minimum Y to 0. Returns the zero vector for an empty
// slice (mirrors boundingBox's empty-input handling).
func centeringOffset(vertices []model.Vector3) model.Vector3 {
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
		if v.Z > max.Z {
			max.Z = v.Z
		}
	}
	return model.Vector3{X: -(min.X + max.X) / 2, Y: -min.Y, Z: -(min.Z + max.Z) / 2}
}

// translateVertices returns a new slice with offset added to every vertex.
// It does not mutate vertices.
func translateVertices(vertices []model.Vector3, offset model.Vector3) []model.Vector3 {
	out := make([]model.Vector3, len(vertices))
	for i, v := range vertices {
		out[i] = model.Vector3{X: v.X + offset.X, Y: v.Y + offset.Y, Z: v.Z + offset.Z}
	}
	return out
}

// boundingBox returns the 8 corners of the axis-aligned bounding box
// enclosing vertices, in a fixed order: the 4 min-Z corners
// (minX,minY)(maxX,minY)(maxX,maxY)(minX,maxY) followed by the same 4 at
// max-Z.
func boundingBox(vertices []model.Vector3) []model.Vector3 {
	if len(vertices) == 0 {
		return make([]model.Vector3, 8)
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
	return []model.Vector3{
		{X: min.X, Y: min.Y, Z: min.Z},
		{X: max.X, Y: min.Y, Z: min.Z},
		{X: max.X, Y: max.Y, Z: min.Z},
		{X: min.X, Y: max.Y, Z: min.Z},
		{X: min.X, Y: min.Y, Z: max.Z},
		{X: max.X, Y: min.Y, Z: max.Z},
		{X: max.X, Y: max.Y, Z: max.Z},
		{X: min.X, Y: max.Y, Z: max.Z},
	}
}

// boxGeometry builds an 8-corner box (in boundingBox's corner order) as 6
// quad faces (one per side, MLOD FaceType 4 - see "P3D Lod Faces.txt"),
// giving a clean 8-point/12-edge wireframe with no triangulation diagonals,
// each with one outward-pointing normal per face-vertex (MLOD/FBX
// convention: normals aren't shared across faces). Winding is verified and
// corrected per quad against the box's own center, rather than hand-derived
// per side, so it's correct regardless of how boundingBox orders its
// corners.
func boxGeometry(corners []model.Vector3) (normals []model.Vector3, faces []model.Face) {
	var center model.Vector3
	for _, c := range corners {
		center.X += c.X / float32(len(corners))
		center.Y += c.Y / float32(len(corners))
		center.Z += c.Z / float32(len(corners))
	}

	quads := [6][4]int{
		{0, 1, 2, 3}, // min-Z side
		{4, 7, 6, 5}, // max-Z side
		{0, 4, 5, 1}, // min-Y side
		{3, 2, 6, 7}, // max-Y side
		{0, 3, 7, 4}, // min-X side
		{1, 5, 6, 2}, // max-X side
	}

	addQuad := func(a, b, c, d int) {
		quad := [4]int{a, b, c, d}
		n := triangleNormal(corners[a], corners[b], corners[c])
		var centroid model.Vector3
		for _, idx := range quad {
			centroid.X += corners[idx].X / 4
			centroid.Y += corners[idx].Y / 4
			centroid.Z += corners[idx].Z / 4
		}
		outward := model.Vector3{X: centroid.X - center.X, Y: centroid.Y - center.Y, Z: centroid.Z - center.Z}
		if dot(n, outward) < 0 {
			quad[0], quad[1], quad[2], quad[3] = quad[3], quad[2], quad[1], quad[0]
			n = triangleNormal(corners[quad[0]], corners[quad[1]], corners[quad[2]])
		}

		base := uint32(len(normals))
		normals = append(normals, n, n, n, n)
		faces = append(faces, model.Face{
			Indices:       []uint32{uint32(quad[0]), uint32(quad[1]), uint32(quad[2]), uint32(quad[3])},
			NormalIndices: []uint32{base, base + 1, base + 2, base + 3},
			UVs:           []model.UV{{}, {}, {}, {}},
		})
	}

	for _, q := range quads {
		addQuad(q[0], q[1], q[2], q[3])
	}

	return normals, faces
}

func triangleNormal(a, b, c model.Vector3) model.Vector3 {
	e1 := model.Vector3{X: b.X - a.X, Y: b.Y - a.Y, Z: b.Z - a.Z}
	e2 := model.Vector3{X: c.X - a.X, Y: c.Y - a.Y, Z: c.Z - a.Z}
	n := model.Vector3{
		X: e1.Y*e2.Z - e1.Z*e2.Y,
		Y: e1.Z*e2.X - e1.X*e2.Z,
		Z: e1.X*e2.Y - e1.Y*e2.X,
	}
	return normalize(n)
}

func dot(a, b model.Vector3) float32 {
	return a.X*b.X + a.Y*b.Y + a.Z*b.Z
}

func normalize(v model.Vector3) model.Vector3 {
	length := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if length == 0 {
		return v
	}
	return model.Vector3{X: v.X / length, Y: v.Y / length, Z: v.Z / length}
}
