package odol

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/p3d/model"
)

type materialBuilder struct {
	t   *testing.T
	buf bytes.Buffer
}

func (b *materialBuilder) write(v any) {
	require.NoError(b.t, binary.Write(&b.buf, binary.LittleEndian, v))
}

func (b *materialBuilder) stage(filter uint32, texture string, texGen uint32) {
	b.write(filter)
	b.buf.WriteString(texture + "\x00")
	b.write(texGen)
	b.buf.WriteByte(0)
}

// dayzMaterialBody builds a type 20 material body (everything after the name
// and type) with two stages and two texgens, the second texgen tiled 2x3 and
// offset by (0.25, 0.5).
func dayzMaterialBody(t *testing.T, stageFilter uint32) []byte {
	b := &materialBuilder{t: t}
	b.write([4]float32{0, 0, 0, 1})            // emissive
	b.write([4]float32{1, 1, 1, 1})            // ambient
	b.write([4]float32{0.5, 0.5, 0.5, 1})      // diffuse
	b.write([4]float32{0, 0, 0, 0})            // forced diffuse
	b.write([4]float32{0.2, 0.2, 0.2, 1})      // specular
	b.write([4]float32{0, 0, 0, 1})            // unknown
	b.write([4]float32{0, 0, 0.3, 0.99})       // unknown
	b.write([4]float32{0.2, 0.2, 0.2, 1})      // specular2
	b.write(float32(60))                       // specular power
	b.buf.Write(make([]byte, 18*4))            // unknown block
	b.write([4]uint32{102, 23, 3, 1})          // pixel/vertex shader, main light, fog mode
	b.buf.WriteString("\x00")                  // surface
	b.write([4]uint32{1, 64, 2, 2})            // unknown, render flags, stage count, texgen count
	b.stage(3, "", 0)                          // Stage0
	b.stage(stageFilter, `dz\x\y_nohq.paa`, 1) // Stage1
	b.write(uint32(1))
	b.write([12]float32{1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0})
	b.write(uint32(2))
	b.write([12]float32{2, 0, 0, 0, 3, 0, 0, 0, 0, 0.25, 0.5, 0})
	b.stage(3, "", 0) // StageTI
	return b.buf.Bytes()
}

// nextMaterialHeader is what follows a material that isn't last in the table.
func nextMaterialHeader() []byte {
	return append([]byte(`dz\x\next.rvmat`+"\x00"), 20, 0, 0, 0)
}

// edgesAndFaceHeader is what follows the last material: two empty edge
// arrays, then a face table header.
func edgesAndFaceHeader() []byte {
	return []byte{
		0, 0, 0, 0, 0, 0, 0, 0, // empty edge arrays
		1, 0, 0, 0, 8, 0, 0, 0, 0, 0, // face count, face data size, zero
		3, 0, 0, 0, 1, 0, 2, 0, // first face
	}
}

func TestDecodeDayZMaterialReadsEveryField(t *testing.T) {
	body := dayzMaterialBody(t, 3)
	p := &parser{data: append(append([]byte(nil), body...), nextMaterialHeader()...)}

	mat, end, ok := p.decodeDayZMaterial(`dz\x\y.rvmat`, 0, false)
	require.True(t, ok)
	assert.Equal(t, len(body), end)

	assert.Equal(t, `dz\x\y.rvmat`, mat.Name)
	assert.Equal(t, uint32(20), mat.Type)
	assert.False(t, mat.Partial)
	assert.Equal(t, [4]float32{0, 0, 0, 1}, mat.Emissive)
	assert.Equal(t, [4]float32{1, 1, 1, 1}, mat.Ambient)
	assert.Equal(t, [4]float32{0.5, 0.5, 0.5, 1}, mat.Diffuse)
	assert.Equal(t, [4]float32{0, 0, 0, 0}, mat.ForcedDiffuse)
	assert.Equal(t, [4]float32{0.2, 0.2, 0.2, 1}, mat.Specular)
	assert.Equal(t, [4]float32{0.2, 0.2, 0.2, 1}, mat.Specular2)
	assert.Equal(t, float32(60), mat.SpecularPower)
	assert.Equal(t, uint32(102), mat.PixelShader)
	assert.Equal(t, uint32(23), mat.VertexShader)
	assert.Equal(t, uint32(3), mat.MainLight)
	assert.Equal(t, uint32(1), mat.FogMode)
	assert.Empty(t, mat.Surface)
	assert.Equal(t, uint32(64), mat.RenderFlags)

	require.Len(t, mat.Stages, 2)
	assert.Equal(t, model.MaterialStage{Filter: 3}, mat.Stages[0])
	assert.Equal(t, model.MaterialStage{Texture: `dz\x\y_nohq.paa`, Filter: 3, TexGen: 1}, mat.Stages[1])

	require.Len(t, mat.TexGens, 2)
	assert.Equal(t, model.TexGen{
		UVSource:  2,
		Transform: [4]model.Vector3{{X: 2}, {Y: 3}, {}, {X: 0.25, Y: 0.5}},
	}, mat.TexGens[1])
	require.NotNil(t, mat.StageTI)
}

func TestDecodeDayZMaterialAcceptsLastMaterialBeforeEdges(t *testing.T) {
	body := dayzMaterialBody(t, 3)
	p := &parser{data: append(append([]byte(nil), body...), edgesAndFaceHeader()...)}

	_, end, ok := p.decodeDayZMaterial(`dz\x\y.rvmat`, 0, true)
	require.True(t, ok)
	assert.Equal(t, len(body), end)
}

func TestDecodeDayZMaterialAllowsTabInStageTexture(t *testing.T) {
	b := &materialBuilder{t: t}
	b.stage(3, "#(argb,8,8,3)color(1,1,1,1,co)\t", 4)
	b.stage(3, "bad\x01path", 0)
	r := &stickyReader{p: &parser{data: b.buf.Bytes()}}

	stage, ok := r.stage()
	require.True(t, ok)
	assert.Equal(t, "#(argb,8,8,3)color(1,1,1,1,co)\t", stage.Texture)

	_, ok = r.stage()
	assert.False(t, ok, "other control characters still reject the stage")
}

func TestDecodeDayZMaterialRejectsBadStage(t *testing.T) {
	body := dayzMaterialBody(t, 9) // no such texture filter
	p := &parser{data: append(append([]byte(nil), body...), nextMaterialHeader()...)}

	_, _, ok := p.decodeDayZMaterial(`dz\x\y.rvmat`, 0, false)
	assert.False(t, ok)
}

func TestDecodeDayZMaterialRejectsWrongSuccessor(t *testing.T) {
	body := dayzMaterialBody(t, 3)

	// A non-last material must be followed by the next material header.
	p := &parser{data: append(append([]byte(nil), body...), edgesAndFaceHeader()...)}
	_, _, ok := p.decodeDayZMaterial(`dz\x\y.rvmat`, 0, false)
	assert.False(t, ok)

	// A truncated body never decodes.
	p = &parser{data: body[:len(body)-10]}
	_, _, ok = p.decodeDayZMaterial(`dz\x\y.rvmat`, 0, true)
	assert.False(t, ok)
}

func TestReadMaterialFallsBackToNameOnly(t *testing.T) {
	header := append([]byte(`dz\x\y.rvmat`+"\x00"), 20, 0, 0, 0)
	junk := make([]byte, 64) // not a decodable body
	data := append(append(append([]byte(nil), header...), junk...), nextMaterialHeader()...)
	p := &parser{data: data}

	mat, err := p.readMaterial(false)
	require.NoError(t, err)
	assert.Equal(t, `dz\x\y.rvmat`, mat.name)
	assert.Nil(t, mat.decoded)
	// The heuristic fallback still lands on the next material's header.
	assert.Equal(t, len(header)+len(junk), p.off)
}

func TestBuildMaterialsMarksUndecodedAsPartial(t *testing.T) {
	decoded := model.Material{Name: "a.rvmat", Type: 20, SpecularPower: 60}
	got := buildMaterials([]rawMaterial{{name: "a.rvmat", decoded: &decoded}, {name: "b.rvmat"}})

	assert.Equal(t, []model.Material{decoded, {Name: "b.rvmat", Partial: true}}, got)
	assert.Nil(t, buildMaterials(nil))
}
