package fbx2p3d

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/p3d/model"
)

func TestTriangulateForWebGLReversesWindingAndFansQuads(t *testing.T) {
	lod := model.LOD{
		Vertices: []model.Vector3{
			{X: 0, Y: 0, Z: 0},
			{X: 1, Y: 0, Z: 0},
			{X: 1, Y: 1, Z: 0},
			{X: 0, Y: 1, Z: 0},
		},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{0, 1, 2, 3}},
		},
	}

	got := TriangulateForWebGL(lod)
	assert.Equal(t, []float32{0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 0}, got.Positions)
	assert.Equal(t, []uint32{
		0, 2, 1, // triangle, clockwise reversed
		0, 2, 1, // quad, first fan triangle from vertex 0
		0, 3, 2, // quad, second fan triangle from vertex 0
	}, got.Triangles)
}

// TestTriangulateForWebGLBoxTrianglesFaceOutward checks the winding against
// real MLOD data: mlodbuild's collision box uses the vanilla clockwise
// convention, so every WebGL triangle must face away from the box center.
func TestTriangulateForWebGLBoxTrianglesFaceOutward(t *testing.T) {
	graphical := mlodbuild.GraphicalLOD{
		Resolution: 1,
		Vertices:   []model.Vector3{{X: -1, Y: 0, Z: -2}, {X: 1, Y: 3, Z: 2}, {X: 0, Y: 1, Z: 0}},
		Faces:      []model.Face{{Indices: []uint32{0, 1, 2}}},
	}
	m, err := mlodbuild.Build([]mlodbuild.GraphicalLOD{graphical}, mlodbuild.Options{})
	require.NoError(t, err)

	geometry := findLOD(t, m, mlodbuild.GeometryResolution)
	mesh := TriangulateForWebGL(geometry)
	require.Len(t, mesh.Triangles, 12*3, "6 quads give 12 triangles")

	var center model.Vector3
	for _, v := range geometry.Vertices {
		center.X += v.X / float32(len(geometry.Vertices))
		center.Y += v.Y / float32(len(geometry.Vertices))
		center.Z += v.Z / float32(len(geometry.Vertices))
	}

	point := func(i uint32) model.Vector3 {
		return model.Vector3{X: mesh.Positions[i*3], Y: mesh.Positions[i*3+1], Z: mesh.Positions[i*3+2]}
	}
	for i := 0; i < len(mesh.Triangles); i += 3 {
		a, b, c := point(mesh.Triangles[i]), point(mesh.Triangles[i+1]), point(mesh.Triangles[i+2])
		e1 := model.Vector3{X: b.X - a.X, Y: b.Y - a.Y, Z: b.Z - a.Z}
		e2 := model.Vector3{X: c.X - a.X, Y: c.Y - a.Y, Z: c.Z - a.Z}
		normal := model.Vector3{
			X: e1.Y*e2.Z - e1.Z*e2.Y,
			Y: e1.Z*e2.X - e1.X*e2.Z,
			Z: e1.X*e2.Y - e1.Y*e2.X,
		}
		centroid := model.Vector3{X: (a.X + b.X + c.X) / 3, Y: (a.Y + b.Y + c.Y) / 3, Z: (a.Z + b.Z + c.Z) / 3}
		outward := model.Vector3{X: centroid.X - center.X, Y: centroid.Y - center.Y, Z: centroid.Z - center.Z}
		dot := normal.X*outward.X + normal.Y*outward.Y + normal.Z*outward.Z
		assert.Greaterf(t, dot, float32(0), "triangle %d faces inward", i/3)
	}
}

// The less detailed LOD comes first, so the highest-detail LOD is not
// simply index 0.
