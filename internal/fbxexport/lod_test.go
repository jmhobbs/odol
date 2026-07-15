package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func TestSelectLOD(t *testing.T) {
	t.Run("single LOD is selected", func(t *testing.T) {
		m := &model.Model{LODs: []model.LOD{{Resolution: 1.0}}}
		lod, err := selectLOD(m)
		require.NoError(t, err)
		assert.Equal(t, float32(1.0), lod.Resolution)
	})

	t.Run("lowest resolution is selected from multiple", func(t *testing.T) {
		m := &model.Model{
			LODs: []model.LOD{
				{Resolution: 2.0},
				{Resolution: 1.0},
				{Resolution: 3.0},
			},
		}
		lod, err := selectLOD(m)
		require.NoError(t, err)
		assert.Equal(t, float32(1.0), lod.Resolution)
	})

	t.Run("first wins on equal resolutions", func(t *testing.T) {
		m := &model.Model{
			LODs: []model.LOD{
				{Resolution: 1.0, Vertices: []model.Vector3{{X: 1}}},
				{Resolution: 1.0, Vertices: []model.Vector3{{X: 2}}},
			},
		}
		lod, err := selectLOD(m)
		require.NoError(t, err)
		assert.Equal(t, float32(1.0), lod.Vertices[0].X)
	})

	t.Run("empty LOD list returns error", func(t *testing.T) {
		m := &model.Model{}
		_, err := selectLOD(m)
		assert.ErrorIs(t, err, errNoLODs)
	})
}
