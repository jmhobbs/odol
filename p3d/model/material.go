package model

// Material.RenderFlags bits. Confirmed by pairing embedded materials with
// their text .rvmat renderFlags[] across P:\DZ (2026-10-21). AlphaTest256 and
// NoReceiveShadow appear in rvmats but set no bit.
const (
	RenderFlagAlwaysInShadow uint32 = 1 << 0
	RenderFlagNoZWrite       uint32 = 1 << 1
	RenderFlagNoColorWrite   uint32 = 1 << 4
	RenderFlagNoAlphaWrite   uint32 = 1 << 5
	RenderFlagAddBlend       uint32 = 1 << 6
	RenderFlagAlphaTest32    uint32 = 1 << 7
	RenderFlagAlphaTest64    uint32 = 1 << 8
	RenderFlagAlphaTest128   uint32 = 1 << 9
	RenderFlagNoTIWrite      uint32 = 1 << 11
)

// TexGen.UVSource values, confirmed the same way.
const (
	UVSourceNone     uint32 = 0
	UVSourceTex      uint32 = 1 // first UV set
	UVSourceTex1     uint32 = 5 // second UV set
	UVSourceWorldPos uint32 = 6
)

// PixelShaderName returns the rvmat PixelShaderID for a Material.PixelShader
// value, or "" when it isn't known.
func PixelShaderName(id uint32) string { return pixelShaderNames[id] }

// VertexShaderName returns the rvmat VertexShaderID for a
// Material.VertexShader value, or "" when it isn't known.
func VertexShaderName(id uint32) string { return vertexShaderNames[id] }

// pixelShaderNames was built from 72,536 embedded/text material pairs in P:\DZ
// (2026-10-21); every id maps to a single name except 0, which binarization
// also writes for materials it couldn't find. The entries marked "docs" were
// never seen embedded in DayZ and come from docs/ODOLV4x.txt (Arma).
var pixelShaderNames = map[uint32]string{
	0:   "Normal",
	2:   "NormalMap", // docs
	5:   "NormalMapDiffuse",
	6:   "Detail",
	12:  "AlphaShadow", // docs
	13:  "AlphaNoShadow",
	15:  "DetailMacroAS",
	18:  "NormalMapSpecularMap",
	19:  "NormalMapDetailSpecularMap",
	22:  "NormalMapSpecularDIMap",
	23:  "NormalMapDetailSpecularDIMap",
	24:  "NormalMapMacroASSpecularDIMap",
	25:  "NormalMapDetailMacroASSpecularDIMap",
	56:  "Glass", // docs
	59:  "Grass",
	102: "Super",
	103: "Multi",
	107: "Tree",
	110: "Skin",
	111: "CalmWater",
	114: "TreeAdv",
	116: "TreeAdvTrunk",
	129: "SuperHair",
}

// vertexShaderNames was built the same way; "docs" entries as above.
var vertexShaderNames = map[uint32]string{
	0:  "Basic",
	1:  "NormalMap",
	2:  "NormalMapDiffuse",
	3:  "Grass",
	14: "BasicAS",
	15: "NormalMapAS",
	17: "Glass", // docs
	23: "Super",
	24: "Multi",
	25: "Tree",
	26: "TreeNoFade",
	29: "Skin",
	30: "CalmWater",
	31: "TreeAdv",
	32: "TreeAdvTrunk",
	35: "TreeAdvModNormals",
}
