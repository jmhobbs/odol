package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShaderNames(t *testing.T) {
	assert.Equal(t, "Super", PixelShaderName(102))
	assert.Equal(t, "Multi", PixelShaderName(103))
	assert.Equal(t, "SuperHair", PixelShaderName(129))
	assert.Equal(t, "", PixelShaderName(9999))

	assert.Equal(t, "Super", VertexShaderName(23))
	assert.Equal(t, "TreeAdvModNormals", VertexShaderName(35))
	assert.Equal(t, "", VertexShaderName(9999))
}

func TestMaterialByName(t *testing.T) {
	lod := LOD{Materials: []Material{{Name: "a.rvmat"}, {Name: "b.rvmat", SpecularPower: 40}}}

	got := lod.MaterialByName("b.rvmat")
	if assert.NotNil(t, got) {
		assert.Equal(t, float32(40), got.SpecularPower)
	}
	assert.Nil(t, lod.MaterialByName("c.rvmat"))
	assert.Nil(t, (&LOD{}).MaterialByName("a.rvmat"))
}
