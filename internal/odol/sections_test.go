package odol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildModelMapsFacesToSectionsByByteOffset(t *testing.T) {
	// Section bounds are byte offsets into the in-memory face array: 8 bytes
	// per triangle, 10 per quad. Section A holds a triangle and a quad
	// (bytes 0-18), section B the triangle after them (bytes 18-26).
	lod := rawLod{
		textures:  []string{"a_co.paa", "b_co.paa"},
		materials: []rawMaterial{{name: "a.rvmat"}, {name: "b.rvmat"}},
		faces: []rawFace{
			{faceType: 3, indices: []uint32{0, 1, 2}},
			{faceType: 4, indices: []uint32{0, 1, 2, 3}},
			{faceType: 3, indices: []uint32{1, 2, 3}},
		},
		sections: []rawSection{
			{faceLower: 0, faceUpper: 18, commonTexture: 0, materialIndex: 0},
			{faceLower: 18, faceUpper: 26, commonTexture: 1, materialIndex: 1},
		},
	}

	m := buildModel("test", 54, rawModelInfo{}, nil, []rawLod{lod})

	require.Len(t, m.LODs, 1)
	faces := m.LODs[0].Faces
	require.Len(t, faces, 3)
	assert.Equal(t, []string{"a_co.paa", "a_co.paa", "b_co.paa"}, []string{faces[0].Texture, faces[1].Texture, faces[2].Texture})
	assert.Equal(t, []string{"a.rvmat", "a.rvmat", "b.rvmat"}, []string{faces[0].Material, faces[1].Material, faces[2].Material})
}

// Sections 0-3 (bytes 0-88) hold the 11 untextured proxy triangles.
