package fbxexport

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// buildDocumentNodes maps a meshScene to the generic FBX node tree shared by
// the ASCII and binary renderers. now is the export timestamp, recorded in
// FBXHeaderExtension/CreationTimeStamp and used by the binary writer to
// derive the file footer code. FileId is generated fresh per call (its
// value is inert - nothing else in the document depends on it, unlike now)
// rather than injected, to avoid threading an unused parameter through
// every call site.
func buildDocumentNodes(s *meshScene, now time.Time) []*node {
	return []*node{
		buildHeaderExtensionNode(now),
		leaf("FileId", randomFileID()),
		leaf("CreationTime", formatCreationTime(now)),
		leaf("Creator", "odol-convert"),
		buildGlobalSettingsNode(),
		buildDocumentsNode(),
		{name: "References", children: []*node{}},
		buildDefinitionsNode(s),
		buildObjectsNode(s),
		buildConnectionsNode(s),
		{name: "Takes", children: []*node{leaf("Current", "")}},
	}
}

// randomFileID generates the 16-byte raw FileId property canonical FBX
// files carry at the top level. Its value isn't validated by any known
// reader (see the footer-code comment in binary.go) - it just needs to be
// present with the right shape.
func randomFileID() []byte {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		panic(fmt.Sprintf("fbxexport: reading random FileId: %v", err))
	}
	return id
}

func formatCreationTime(now time.Time) string {
	return fmt.Sprintf("%s:%03d", now.Format("2006-01-02 15:04:05"), now.Nanosecond()/1_000_000)
}

func leaf(name string, props ...any) *node {
	return &node{name: name, props: props}
}

func p(props ...any) *node {
	return &node{name: "P", props: props}
}

func buildHeaderExtensionNode(now time.Time) *node {
	return &node{
		name: "FBXHeaderExtension",
		children: []*node{
			leaf("FBXHeaderVersion", int32(1004)),
			leaf("FBXVersion", int32(fbxBinaryVersion)),
			leaf("EncryptionType", int32(0)),
			{
				name: "CreationTimeStamp",
				children: []*node{
					leaf("Version", int32(1000)),
					leaf("Year", int32(now.Year())),
					leaf("Month", int32(now.Month())),
					leaf("Day", int32(now.Day())),
					leaf("Hour", int32(now.Hour())),
					leaf("Minute", int32(now.Minute())),
					leaf("Second", int32(now.Second())),
					leaf("Millisecond", int32(now.Nanosecond()/1_000_000)),
				},
			},
			leaf("Creator", "odol-convert"),
			{name: "OtherFlags", children: []*node{leaf("TCDefinition", int32(127))}},
		},
	}
}

func buildGlobalSettingsNode() *node {
	return &node{
		name: "GlobalSettings",
		children: []*node{
			leaf("Version", int32(1000)),
			{
				name: "Properties70",
				children: []*node{
					p("UpAxis", "int", "Integer", "", int32(1)),
					p("UpAxisSign", "int", "Integer", "", int32(1)),
					p("FrontAxis", "int", "Integer", "", int32(2)),
					p("FrontAxisSign", "int", "Integer", "", int32(1)),
					p("CoordAxis", "int", "Integer", "", int32(0)),
					p("CoordAxisSign", "int", "Integer", "", int32(1)),
					// 1 means "1 raw unit = 1 cm", the standard FBX
					// convention. Vertex positions (build.go) are scaled
					// from ODOL/MLOD's native meters into centimeters at
					// export time to make that true, so output is
					// real-world-accurate with no consumer-side workaround.
					// A deliberate departure from canonical DayZ FBX SDK
					// output, which declares this same value while leaving
					// vertex data in meters - see plan_fbx-realworld-scale.md
					// (reversing the meters-for-canonical-parity decision in
					// plan_fbx-parity.md) for why, including the empirical
					// finding that consumers don't apply any correction
					// based on this property's value alone.
					p("UnitScaleFactor", "double", "Number", "", float64(1)),
				},
			},
		},
	}
}

func buildDocumentsNode() *node {
	return &node{
		name: "Documents",
		children: []*node{
			leaf("Count", int32(1)),
			{
				name:     "Document",
				props:    []any{int64(1000000000), "", "Scene"},
				children: []*node{leaf("RootNode", int64(0))},
			},
		},
	}
}

func buildDefinitionsNode(s *meshScene) *node {
	modelCount := len(s.parts) + len(s.proxies) + len(s.selections)
	matCount := len(s.materials)

	objectTypeCount := 3
	if matCount > 0 {
		objectTypeCount += 3 // Material, Texture, Video
	}

	children := []*node{
		leaf("Version", int32(100)),
		leaf("Count", int32(objectTypeCount)),
		objectTypeNode("GlobalSettings", 1),
		objectTypeNode("Geometry", len(s.parts)),
		objectTypeNode("Model", modelCount),
	}
	if matCount > 0 {
		children = append(children,
			objectTypeNode("Material", matCount),
			objectTypeNode("Texture", matCount),
			objectTypeNode("Video", matCount),
		)
	}

	return &node{name: "Definitions", children: children}
}

func objectTypeNode(typeName string, count int) *node {
	return &node{
		name:     "ObjectType",
		props:    []any{typeName},
		children: []*node{leaf("Count", int32(count))},
	}
}

func buildObjectsNode(s *meshScene) *node {
	var children []*node
	for i := range s.parts {
		part := &s.parts[i]
		children = append(children, buildGeometryNode(part), buildMeshModelNode(s, part))
	}
	for i := range s.materials {
		mat := &s.materials[i]
		children = append(children, buildMaterialNode(mat), buildVideoNode(mat), buildTextureNode(mat))
	}
	for i := range s.proxies {
		children = append(children, buildProxyModelNode(&s.proxies[i]))
	}
	for i := range s.selections {
		children = append(children, buildSelectionModelNode(&s.selections[i]))
	}
	return &node{name: "Objects", children: children}
}

func buildGeometryNode(part *meshPart) *node {
	children := []*node{
		leaf("Vertices", part.geometry.vertices),
		leaf("PolygonVertexIndex", part.geometry.polyIndex),
		leaf("GeometryVersion", int32(124)),
		{
			name:  "LayerElementNormal",
			props: []any{int32(0)},
			children: []*node{
				leaf("Version", int32(102)),
				leaf("Name", ""),
				leaf("MappingInformationType", "ByPolygonVertex"),
				leaf("ReferenceInformationType", "Direct"),
				leaf("Normals", part.geometry.normals),
			},
		},
	}

	for i, uvChannel := range part.geometry.uvSets {
		children = append(children, &node{
			name:  "LayerElementUV",
			props: []any{int32(i)},
			children: []*node{
				leaf("Version", int32(101)),
				leaf("Name", fmt.Sprintf("UVChannel_%d", i+1)),
				leaf("MappingInformationType", "ByPolygonVertex"),
				leaf("ReferenceInformationType", "Direct"),
				leaf("UV", uvChannel),
			},
		})
	}

	hasMaterials := len(part.localMaterials) > 0
	if hasMaterials {
		children = append(children, &node{
			name:  "LayerElementMaterial",
			props: []any{int32(0)},
			children: []*node{
				leaf("Version", int32(101)),
				leaf("Name", ""),
				leaf("MappingInformationType", "ByPolygon"),
				leaf("ReferenceInformationType", "IndexToDirect"),
				leaf("Materials", part.geometry.matIndex),
			},
		})
	}

	layerChildren := []*node{
		leaf("Version", int32(100)),
		{name: "LayerElement", children: []*node{
			leaf("Type", "LayerElementNormal"),
			leaf("TypedIndex", int32(0)),
		}},
	}
	for i := range part.geometry.uvSets {
		layerChildren = append(layerChildren, &node{name: "LayerElement", children: []*node{
			leaf("Type", "LayerElementUV"),
			leaf("TypedIndex", int32(i)),
		}})
	}
	if hasMaterials {
		layerChildren = append(layerChildren, &node{name: "LayerElement", children: []*node{
			leaf("Type", "LayerElementMaterial"),
			leaf("TypedIndex", int32(0)),
		}})
	}
	children = append(children, &node{name: "Layer", props: []any{int32(0)}, children: layerChildren})

	return &node{
		name:     "Geometry",
		props:    []any{part.geomID, fmt.Sprintf("Geometry::%s", part.name), "Mesh"},
		children: children,
	}
}

func buildMeshModelNode(s *meshScene, part *meshPart) *node {
	props70 := []*node{
		p("ScalingMax", "Vector3D", "Vector", "", int32(0), int32(0), int32(0)),
		p("DefaultAttributeIndex", "int", "Integer", "", int32(0)),
		p("ODOL_SourceFamily", "KString", "", "", s.sourceFamily),
		p("ODOL_SourceVersion", "int", "Integer", "", int64(s.sourceVersion)),
		p("ODOL_LODResolution", "double", "Number", "", float64(s.lodResolution)),
		p("ODOL_IconColor", "int", "Integer", "", int64(s.iconColor)),
		p("ODOL_SelectedColor", "int", "Integer", "", int64(s.selectedColor)),
	}
	for _, k := range s.odolPropertyKeys {
		props70 = append(props70, p("ODOL_Prop_"+k, "KString", "", "", s.odolProperties[k]))
	}

	return &node{
		name:  "Model",
		props: []any{part.modelID, fmt.Sprintf("Model::%s", part.name), "Mesh"},
		children: []*node{
			leaf("Version", int32(232)),
			{name: "Properties70", children: props70},
			leaf("Shading", shadingFlag(true)),
			leaf("Culling", "CullingOff"),
		},
	}
}

// buildMaterialNode emits a phong-shaded Material. The Emissive/Ambient/
// Diffuse/Specular/Shininess/Opacity/Reflectivity values are generic phong
// defaults (matching canonical DayZ FBX SDK output), not real .rvmat color
// data - ODOL doesn't carry parsed rvmat contents, only the material's file
// path (kept below as ODOL_Material for traceability). This is a deliberate
// deviation from strict parity: keeping the ODOL_Texture/ODOL_Material
// properties alongside the real phong properties preserves ODOL provenance
// that canonical exports don't carry.
func buildMaterialNode(mat *materialData) *node {
	return &node{
		name:  "Material",
		props: []any{mat.id, fmt.Sprintf("Material::%s", mat.name), ""},
		children: []*node{
			leaf("Version", int32(102)),
			leaf("ShadingModel", "phong"),
			leaf("MultiLayer", int32(0)),
			{name: "Properties70", children: []*node{
				p("Emissive", "Vector3D", "Vector", "", float64(0), float64(0), float64(0)),
				p("Ambient", "Vector3D", "Vector", "", 0.2, 0.2, 0.2),
				p("Diffuse", "Vector3D", "Vector", "", 0.8, 0.8, 0.8),
				p("Specular", "Vector3D", "Vector", "", 0.2, 0.2, 0.2),
				p("Shininess", "double", "Number", "", float64(20)),
				p("Opacity", "double", "Number", "", float64(1)),
				p("Reflectivity", "double", "Number", "", float64(0)),
				p("ODOL_Texture", "KString", "", "", mat.texture),
				p("ODOL_Material", "KString", "", "", mat.mat),
			}},
		},
	}
}

// buildVideoNode emits the Video "Clip" object that a Texture object
// references for its image data. Real DayZ exports point RelativeFilename
// at the exporting tool's own install path (an artifact of that specific
// tool, not a meaningful convention) - this uses the material's real ODOL
// texture path instead, which is actually useful to a consumer.
func buildVideoNode(mat *materialData) *node {
	return &node{
		name:  "Video",
		props: []any{mat.videoID, fmt.Sprintf("Video::%s_Video", mat.name), "Clip"},
		children: []*node{
			leaf("Type", "Clip"),
			{name: "Properties70", children: []*node{
				p("RelPath", "KString", "XRefUrl", "", mat.texture),
			}},
			leaf("UseMipMap", int32(0)),
			leaf("Filename", ""),
			leaf("RelativeFilename", mat.texture),
		},
	}
}

// buildTextureNode emits the Texture object a Material's DiffuseColor
// connects to (see buildConnectionsNode). Shares its object name with the
// Material, matching canonical DayZ FBX exports.
func buildTextureNode(mat *materialData) *node {
	return &node{
		name:  "Texture",
		props: []any{mat.textureID, fmt.Sprintf("Texture::%s", mat.name), ""},
		children: []*node{
			leaf("Type", "TextureVideoClip"),
			leaf("Version", int32(202)),
			leaf("TextureName", fmt.Sprintf("Texture::%s", mat.name)),
			{name: "Properties70", children: []*node{
				p("UVSet", "KString", "", "", "UVChannel_1"),
			}},
			leaf("Media", fmt.Sprintf("Video::%s_Video", mat.name)),
			leaf("FileName", ""),
			leaf("RelativeFilename", mat.texture),
			leaf("ModelUVTranslation", float64(0), float64(0)),
			leaf("ModelUVScaling", float64(1), float64(1)),
			leaf("Texture_Alpha_Source", "None"),
			leaf("Cropping", int32(0), int32(0), int32(0), int32(0)),
		},
	}
}

func buildProxyModelNode(proxy *proxyData) *node {
	return &node{
		name:  "Model",
		props: []any{proxy.id, fmt.Sprintf("Model::%s", proxy.name), "Null"},
		children: []*node{
			leaf("Version", int32(232)),
			{name: "Properties70", children: []*node{
				p("ODOL_ProxyPath", "KString", "", "", proxy.path),
				p("ODOL_ProxySequenceID", "int", "Integer", "", int64(proxy.sequenceID)),
				p("ODOL_ProxySelectionName", "KString", "", "", proxy.selectionName),
				p("ODOL_ProxySectionIndex", "int", "Integer", "", int64(proxy.sectionIndex)),
			}},
			leaf("Shading", shadingFlag(true)),
			leaf("Culling", "CullingOff"),
		},
	}
}

func buildSelectionModelNode(sel *selectionData) *node {
	isSectionalInt := int64(0)
	if sel.isSectional {
		isSectionalInt = 1
	}
	return &node{
		name:  "Model",
		props: []any{sel.id, fmt.Sprintf("Model::%s", sel.name), "Null"},
		children: []*node{
			leaf("Version", int32(232)),
			{name: "Properties70", children: []*node{
				p("ODOL_SelectionIsSectional", "bool", "", "", isSectionalInt),
				p("ODOL_SelectionVertexIndices", "KString", "", "", joinUint32s(sel.vertexIndices)),
				p("ODOL_SelectionVertexWeights", "KString", "", "", joinBytes(sel.vertexWeights)),
				p("ODOL_SelectionFaceIndices", "KString", "", "", joinUint32s(sel.faceIndices)),
			}},
			leaf("Shading", shadingFlag(true)),
			leaf("Culling", "CullingOff"),
		},
	}
}

func buildConnectionsNode(s *meshScene) *node {
	var children []*node
	for i := range s.parts {
		part := &s.parts[i]
		children = append(children, leaf("C", "OO", part.geomID, part.modelID))
		children = append(children, leaf("C", "OO", part.modelID, int64(0)))
		for _, globalIdx := range part.localMaterials {
			mat := s.materials[globalIdx]
			children = append(children, leaf("C", "OO", mat.id, part.modelID))
		}
	}
	for _, mat := range s.materials {
		children = append(children,
			leaf("C", "OO", mat.videoID, mat.textureID),
			leaf("C", "OP", mat.textureID, mat.id, "DiffuseColor"),
		)
	}
	for _, proxy := range s.proxies {
		children = append(children, leaf("C", "OO", proxy.id, int64(0)))
	}
	for _, sel := range s.selections {
		children = append(children, leaf("C", "OO", sel.id, int64(0)))
	}
	return &node{name: "Connections", children: children}
}

// joinUint32s renders a uint32 slice as a comma-separated string, used for
// custom string properties (not real FBX array properties).
func joinUint32s(values []uint32) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	return sb.String()
}

// joinBytes renders a byte slice as a comma-separated string of decimal
// values, used for custom string properties.
func joinBytes(values []byte) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	return sb.String()
}
