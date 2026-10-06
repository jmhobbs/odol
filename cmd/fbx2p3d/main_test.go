package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func TestParseLODFlagRejectsEmptyPath(t *testing.T) {
	_, err := parseLODFlag("1.0=")
	assert.Error(t, err)
}

func TestOutputPathExplicitOverride(t *testing.T) {
	lods := []lodEntry{{Resolution: 1.0, Path: "a.fbx"}}
	assert.Equal(t, "out.p3d", outputPath(lods, "out.p3d"))
}

func TestOutputPathDerivedFromLowestResolution(t *testing.T) {
	lods := []lodEntry{
		{Resolution: 2.0, Path: filepath.Join("dir", "low.fbx")},
		{Resolution: 1.0, Path: filepath.Join("dir", "high.fbx")},
	}
	assert.Equal(t, filepath.Join("dir", "high.p3d"), outputPath(lods, ""))
}

func TestLODFlagListSetAccumulates(t *testing.T) {
	var l lodFlagList
	require.NoError(t, l.Set("1.0=a.fbx"))
	require.NoError(t, l.Set("2.0=b.fbx"))
	require.Len(t, l, 2)
	assert.Equal(t, float32(1.0), l[0].Resolution)
	assert.Equal(t, float32(2.0), l[1].Resolution)
}

func TestScaleVerticesIdentity(t *testing.T) {
	in := []model.Vector3{{X: 1, Y: 2, Z: 3}, {X: -4, Y: 5, Z: -6}}
	got := scaleVertices(in, 1.0)
	assert.Equal(t, in, got)
}

func TestScaleVerticesNonTrivialFactor(t *testing.T) {
	in := []model.Vector3{{X: 1, Y: 2, Z: 3}, {X: -4, Y: 5, Z: -6}}
	got := scaleVertices(in, 2.0)
	assert.Equal(t, []model.Vector3{{X: 2, Y: 4, Z: 6}, {X: -8, Y: 10, Z: -12}}, got)

	got = scaleVertices(in, 0.01)
	require.Len(t, got, 2)
	assert.InDelta(t, 0.01, got[0].X, 1e-6)
	assert.InDelta(t, 0.02, got[0].Y, 1e-6)
	assert.InDelta(t, 0.03, got[0].Z, 1e-6)
}

func TestScaleVerticesDoesNotMutateInput(t *testing.T) {
	in := []model.Vector3{{X: 1, Y: 1, Z: 1}}
	_ = scaleVertices(in, 5.0)
	assert.Equal(t, model.Vector3{X: 1, Y: 1, Z: 1}, in[0], "scaleVertices must not mutate its input slice")
}

func TestValidateScaleRejectsZeroAndNegative(t *testing.T) {
	assert.Error(t, validateScale(0))
	assert.Error(t, validateScale(-1))
	assert.Error(t, validateScale(-0.001))
}

func TestValidateScaleAcceptsPositive(t *testing.T) {
	assert.NoError(t, validateScale(1.0))
	assert.NoError(t, validateScale(0.01))
	assert.NoError(t, validateScale(100))
}
