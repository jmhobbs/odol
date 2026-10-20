package fbx2p3d

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/internal/model"
)

// fileSource returns a LODSource that reads the sample at path when loaded,
// the same way cmd/fbx2p3d reads --lod files.

func defaultOptions() mlodbuild.Options {
	return mlodbuild.Options{
		Texture:  "ca\\camoground.paa",
		Material: "ca\\camoground.rvmat",
		Mass:     80,
	}
}

func findLOD(t *testing.T, m *model.Model, resolution float32) model.LOD {
	t.Helper()
	for _, lod := range m.LODs {
		if lod.Resolution == resolution {
			return lod
		}
	}
	t.Fatalf("no LOD with resolution %g", resolution)
	return model.LOD{}
}

func TestBuildWrapsImportErrorWithSourceName(t *testing.T) {
	source := LODSource{
		Resolution: 1.0,
		Name:       "broken.fbx",
		Load:       func() ([]byte, error) { return []byte("not an fbx"), nil },
	}

	_, err := Build([]LODSource{source}, defaultOptions(), 1.0, Hooks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.fbx: ")
}

func TestBoundingBoxSize(t *testing.T) {
	vertices := []model.Vector3{
		{X: -1, Y: 0, Z: -2},
		{X: 1, Y: 3, Z: 2},
		{X: 0, Y: -1, Z: 0},
	}
	got := BoundingBoxSize(vertices)
	assert.Equal(t, model.Vector3{X: 2, Y: 4, Z: 4}, got)
}

func TestBoundingBoxSizeEmpty(t *testing.T) {
	assert.Equal(t, model.Vector3{}, BoundingBoxSize(nil))
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
	assert.Error(t, ValidateScale(0))
	assert.Error(t, ValidateScale(-1))
	assert.Error(t, ValidateScale(-0.001))
}

func TestValidateScaleAcceptsPositive(t *testing.T) {
	assert.NoError(t, ValidateScale(1.0))
	assert.NoError(t, ValidateScale(0.01))
	assert.NoError(t, ValidateScale(100))
}
