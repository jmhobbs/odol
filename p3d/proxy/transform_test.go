package proxy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/p3d/model"
)

func assertTransformNear(t *testing.T, want, got [4]model.Vector3, msg string) {
	t.Helper()
	for i := range want {
		assert.InDelta(t, want[i].X, got[i].X, 1e-3, "%s row %d X", msg, i)
		assert.InDelta(t, want[i].Y, got[i].Y, 1e-3, "%s row %d Y", msg, i)
		assert.InDelta(t, want[i].Z, got[i].Z, 1e-3, "%s row %d Z", msg, i)
	}
}

func TestTransformFromTriangleUsesLegsAsAxes(t *testing.T) {
	// Right angle at (1,2,3): short leg along +Z, long leg along +X.
	vertices := []model.Vector3{{X: 1, Y: 2, Z: 3}, {X: 1, Y: 2, Z: 3.2}, {X: 1.4, Y: 2, Z: 3}}
	want := [4]model.Vector3{
		{Y: -1},            // X = Y x Z
		{X: 1},             // long leg
		{Z: 1},             // short leg
		{X: 1, Y: 2, Z: 3}, // right-angle corner
	}

	for _, indices := range [][]uint32{{0, 1, 2}, {1, 2, 0}, {2, 0, 1}, {0, 2, 1}} {
		got, ok := TransformFromTriangle(vertices, indices)
		require.True(t, ok, "indices %v", indices)
		assertTransformNear(t, want, got, fmt.Sprint("indices ", indices))
	}
}

func TestTransformFromTriangleRejectsOtherShapes(t *testing.T) {
	vertices := []model.Vector3{{}, {X: 1}, {Y: 1}, {X: 1, Y: 1, Z: 1}}

	_, ok := TransformFromTriangle(vertices, []uint32{0, 1, 2}) // right angle, legs 1:1
	assert.False(t, ok)
	_, ok = TransformFromTriangle(vertices, []uint32{0, 1, 3}) // no right angle
	assert.False(t, ok)
	_, ok = TransformFromTriangle(vertices, []uint32{0, 1, 2, 3}) // quad
	assert.False(t, ok)
	_, ok = TransformFromTriangle(vertices, []uint32{0, 1, 9}) // index out of range
	assert.False(t, ok)
}
