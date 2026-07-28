package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func faceOf(indices ...uint32) model.Face {
	return model.Face{Indices: indices}
}

func TestGroupConnectedComponentsDisjointFaces(t *testing.T) {
	faces := []model.Face{
		faceOf(0, 1, 2),
		faceOf(3, 4, 5),
	}
	groups := groupConnectedComponents(faces, 6)

	require.Len(t, groups, 2)
	assert.Equal(t, []int{0}, groups[0].faceIndices)
	assert.Equal(t, []int{1}, groups[1].faceIndices)
}

func TestGroupConnectedComponentsSharedVertexMerges(t *testing.T) {
	faces := []model.Face{
		faceOf(0, 1, 2),
		faceOf(2, 3, 4), // shares vertex 2 with the first face
	}
	groups := groupConnectedComponents(faces, 5)

	require.Len(t, groups, 1)
	assert.Equal(t, []int{0, 1}, groups[0].faceIndices)
}

func TestGroupConnectedComponentsOrderedByFirstFace(t *testing.T) {
	faces := []model.Face{
		faceOf(3, 4, 5), // component A, appears first
		faceOf(0, 1, 2), // component B, appears second
		faceOf(3, 5, 4), // still component A
	}
	groups := groupConnectedComponents(faces, 6)

	require.Len(t, groups, 2)
	assert.Equal(t, []int{0, 2}, groups[0].faceIndices, "component containing the first face should sort first")
	assert.Equal(t, []int{1}, groups[1].faceIndices)
}

func TestGroupConnectedComponentsTransitiveChain(t *testing.T) {
	// A-B-C chain: face0 and face1 share vertex 1, face1 and face2 share
	// vertex 2 - all three must end up in the same component even though
	// face0 and face2 share no vertex directly.
	faces := []model.Face{
		faceOf(0, 1),
		faceOf(1, 2),
		faceOf(2, 3),
	}
	groups := groupConnectedComponents(faces, 4)

	require.Len(t, groups, 1)
	assert.Equal(t, []int{0, 1, 2}, groups[0].faceIndices)
}

func TestGroupConnectedComponentsEmpty(t *testing.T) {
	groups := groupConnectedComponents(nil, 0)
	assert.Empty(t, groups)
}

func TestGroupConnectedComponentsSkipsEmptyFaces(t *testing.T) {
	faces := []model.Face{
		{Indices: nil},
		faceOf(0, 1, 2),
	}
	groups := groupConnectedComponents(faces, 3)

	require.Len(t, groups, 1)
	assert.Equal(t, []int{1}, groups[0].faceIndices)
}
