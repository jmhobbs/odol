package mlod

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/jmhobbs/odol/internal/model"
)

const (
	mlodVersion      = 0x101
	mlodMajorVersion = 28
	mlodMinorVersion = 0x100
)

func WriteFile(path string, m *model.Model) error {
	data, err := Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Marshal(m *model.Model) ([]byte, error) {
	var out bytes.Buffer

	writeRaw(&out, []byte("MLOD"))
	writeU32(&out, mlodVersion)
	writeU32(&out, uint32(len(m.LODs)))

	for _, lod := range m.LODs {
		if err := writeLOD(&out, lod); err != nil {
			return nil, err
		}
	}

	return out.Bytes(), nil
}

func writeLOD(w io.Writer, lod model.LOD) error {
	writeRaw(w, []byte("P3DM"))
	writeU32(w, mlodMajorVersion)
	writeU32(w, mlodMinorVersion)
	writeU32(w, uint32(len(lod.Vertices)))
	writeU32(w, uint32(len(lod.Normals)))
	writeU32(w, uint32(len(lod.Faces)))
	writeU32(w, 0)

	for _, vertex := range lod.Vertices {
		writeVec3(w, vertex)
		writeU32(w, 0)
	}
	for _, normal := range lod.Normals {
		writeVec3(w, normal)
	}
	for _, face := range lod.Faces {
		if err := writeFace(w, face); err != nil {
			return err
		}
	}

	writeRaw(w, []byte("TAGG"))
	if len(lod.PointMasses) == len(lod.Vertices) && len(lod.PointMasses) > 0 {
		writeTagg(w, "#Mass#", marshalMasses(lod.PointMasses))
	}
	writeTagg(w, "#UVSet#", marshalUVSet(0, marshalDefaultUVSet(lod)))
	for id, uvSet := range lod.UVSets {
		if id == 0 {
			continue
		}
		writeTagg(w, "#UVSet#", marshalUVSet(uint32(id), marshalIndexedUVSet(lod, uvSet)))
	}

	propertyKeys := make([]string, 0, len(lod.Properties))
	seenProperties := make(map[string]struct{}, len(lod.Properties))
	for _, key := range lod.PropertyOrder {
		if _, ok := lod.Properties[key]; !ok {
			continue
		}
		if _, ok := seenProperties[key]; ok {
			continue
		}
		propertyKeys = append(propertyKeys, key)
		seenProperties[key] = struct{}{}
	}
	extraPropertyKeys := make([]string, 0, len(lod.Properties)-len(propertyKeys))
	for key := range lod.Properties {
		if _, ok := seenProperties[key]; ok {
			continue
		}
		extraPropertyKeys = append(extraPropertyKeys, key)
	}
	sort.Strings(extraPropertyKeys)
	propertyKeys = append(propertyKeys, extraPropertyKeys...)
	for _, key := range propertyKeys {
		writeTagg(w, "#Property#", marshalProperty(key, lod.Properties[key]))
	}

	for _, selection := range lod.Selections {
		writeTagg(w, selection.Name, marshalSelection(lod, selection))
	}
	writeTagg(w, "#EndOfFile#", nil)
	writeF32(w, lod.Resolution)

	return nil
}

func writeFace(w io.Writer, face model.Face) error {
	if len(face.Indices) != 3 && len(face.Indices) != 4 {
		return fmt.Errorf("unsupported face vertex count %d", len(face.Indices))
	}

	writeU32(w, uint32(len(face.Indices)))
	count := len(face.Indices)
	for i := 0; i < 4; i++ {
		var pointIndex uint32
		var normalIndex uint32
		var uv model.UV
		if i < count {
			pointIndex = face.Indices[i]
			if i < len(face.NormalIndices) {
				normalIndex = face.NormalIndices[i]
			}
			if i < len(face.UVs) {
				uv = face.UVs[i]
			}
		}
		writeU32(w, pointIndex)
		writeU32(w, normalIndex)
		writeF32(w, uv.U)
		writeF32(w, uv.V)
	}
	writeU32(w, face.Flags)
	writeString(w, face.Texture)
	writeString(w, face.Material)

	return nil
}

func marshalProperty(key, value string) []byte {
	buf := make([]byte, 128)
	copy(buf[:64], append([]byte(key), 0))
	copy(buf[64:], append([]byte(value), 0))
	return buf
}

func marshalSelection(lod model.LOD, selection model.Selection) []byte {
	pointWeights := make([]byte, len(lod.Vertices))
	for i, index := range selection.VertexIndices {
		if int(index) >= len(pointWeights) {
			continue
		}
		weight := byte(1)
		if i < len(selection.VertexWeights) && selection.VertexWeights[i] != 0 {
			weight = selection.VertexWeights[i]
		}
		pointWeights[index] = weight
	}

	faces := make([]byte, len(lod.Faces))
	for _, index := range selection.FaceIndices {
		if int(index) < len(faces) {
			faces[index] = 1
			for _, vertexIndex := range lod.Faces[index].Indices {
				if int(vertexIndex) < len(pointWeights) && pointWeights[vertexIndex] == 0 {
					pointWeights[vertexIndex] = 1
				}
			}
		}
	}

	return append(pointWeights, faces...)
}

func marshalUVSet(id uint32, uvSet []byte) []byte {
	var out bytes.Buffer

	writeU32(&out, id)
	writeRaw(&out, uvSet)

	return out.Bytes()
}

func marshalDefaultUVSet(lod model.LOD) []byte {
	var out bytes.Buffer
	for _, face := range lod.Faces {
		for _, uv := range face.UVs {
			writeF32(&out, uv.U)
			writeF32(&out, uv.V)
		}
	}
	return out.Bytes()
}

func marshalIndexedUVSet(lod model.LOD, uvSet []model.UV) []byte {
	var out bytes.Buffer
	for _, face := range lod.Faces {
		for _, index := range face.Indices {
			var uv model.UV
			if int(index) < len(uvSet) {
				uv = uvSet[index]
			}
			writeF32(&out, uv.U)
			writeF32(&out, uv.V)
		}
	}
	return out.Bytes()
}

func marshalMasses(pointMasses []float32) []byte {
	var out bytes.Buffer
	for _, mass := range pointMasses {
		writeF32(&out, mass)
	}
	return out.Bytes()
}

func writeTagg(w io.Writer, name string, data []byte) {
	writeU8(w, 1)
	writeString(w, name)
	writeU32(w, uint32(len(data)))
	writeRaw(w, data)
}

func writeString(w io.Writer, value string) {
	writeRaw(w, append([]byte(value), 0))
}

func writeVec3(w io.Writer, v model.Vector3) {
	writeF32(w, v.X)
	writeF32(w, v.Y)
	writeF32(w, v.Z)
}

func writeU8(w io.Writer, value uint8) {
	writeRaw(w, []byte{value})
}

func writeU32(w io.Writer, value uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], value)
	writeRaw(w, buf[:])
}

func writeF32(w io.Writer, value float32) {
	writeU32(w, math.Float32bits(value))
}

func writeRaw(w io.Writer, data []byte) {
	_, _ = w.Write(data)
}
