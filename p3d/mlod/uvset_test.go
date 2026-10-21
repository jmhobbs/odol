package mlod

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/p3d/model"
)

// uvData encodes UV pairs the way a #UVSet# TAGG stores them after its id.
func uvData(t *testing.T, uvs ...model.UV) []byte {
	var buf bytes.Buffer
	for _, uv := range uvs {
		require.NoError(t, binary.Write(&buf, binary.LittleEndian, [2]float32{uv.U, uv.V}))
	}
	return buf.Bytes()
}

func TestAssignFaceUVSetKeepsSeams(t *testing.T) {
	// Both faces use vertex 0, with a different set-1 UV each: a seam.
	faces := []model.Face{
		{Indices: []uint32{0, 1, 2}},
		{Indices: []uint32{0, 2, 3}},
	}
	data := uvData(t,
		model.UV{U: 0.1, V: 0.2}, model.UV{U: 0.3, V: 0.4}, model.UV{U: 0.5, V: 0.6},
		model.UV{U: 0.9, V: 0.9}, model.UV{U: 0.5, V: 0.6}, model.UV{U: 0.7, V: 0.8},
	)

	assignFaceUVSet(data, faces, 1)

	assert.Equal(t, [][]model.UV{nil, {{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}}}, faces[0].UVSets)
	assert.Equal(t, [][]model.UV{nil, {{U: 0.9, V: 0.9}, {U: 0.5, V: 0.6}, {U: 0.7, V: 0.8}}}, faces[1].UVSets)

	// The per-vertex copy can only keep vertex 0's first value.
	perVertex := reconstructUVSet(data, faces, 4)
	assert.Equal(t, model.UV{U: 0.1, V: 0.2}, perVertex[0])
}

func TestAssignFaceUVSetStopsAtShortData(t *testing.T) {
	faces := []model.Face{
		{Indices: []uint32{0, 1, 2}},
		{Indices: []uint32{0, 2, 3}},
	}
	data := uvData(t, model.UV{U: 0.1}, model.UV{U: 0.2}, model.UV{U: 0.3}, model.UV{U: 0.4})

	assignFaceUVSet(data, faces, 2)

	require.Len(t, faces[0].UVSets, 3)
	assert.Equal(t, []model.UV{{U: 0.1}, {U: 0.2}, {U: 0.3}}, faces[0].UVSets[2])
	assert.Nil(t, faces[1].UVSets, "a face with only part of its data gets no entry")
}

func TestParseFillsFaceUVSets(t *testing.T) {
	m := minimalModel()
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}},
	}

	parsed := marshalAndParse(t, m)

	face := parsed.LODs[0].Faces[0]
	require.Len(t, face.UVSets, 2)
	assert.Nil(t, face.UVSets[0])
	want := make([]model.UV, len(face.Indices))
	for i, vi := range face.Indices {
		want[i] = m.LODs[0].UVSets[1][vi]
	}
	assert.Equal(t, want, face.UVSets[1])
}
