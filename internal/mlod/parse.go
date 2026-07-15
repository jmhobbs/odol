package mlod

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmhobbs/odol/internal/model"
)

// ParseFile reads an MLOD P3D file from disk and parses it.
// The model name is derived from the filename without extension.
func ParseFile(path string) (*model.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return Parse(data, name)
}

// Parse parses MLOD binary data with the given model name.
func Parse(data []byte, name string) (*model.Model, error) {
	r := &mlodReader{data: data}

	r.expectMagic("MLOD")
	if r.err != nil {
		return nil, fmt.Errorf("not an MLOD file: %w", r.err)
	}

	version := r.readU32()
	lodCount := r.readU32()
	if r.err != nil {
		return nil, r.err
	}

	lods := make([]model.LOD, 0, lodCount)
	for i := uint32(0); i < lodCount; i++ {
		lod, err := parseLOD(r)
		if err != nil {
			return nil, fmt.Errorf("LOD %d: %w", i, err)
		}
		lods = append(lods, lod)
	}

	return &model.Model{
		Name:   name,
		Source: model.SourceFormat{Family: "MLOD", Version: version},
		LODs:   lods,
	}, nil
}

func parseLOD(r *mlodReader) (model.LOD, error) {
	r.expectMagic("P3DM")
	if r.err != nil {
		return model.LOD{}, fmt.Errorf("expected P3DM header: %w", r.err)
	}

	_ = r.readU32() // major version
	_ = r.readU32() // minor version
	vertexCount := r.readU32()
	normalCount := r.readU32()
	faceCount := r.readU32()
	_ = r.readU32() // flags
	if r.err != nil {
		return model.LOD{}, r.err
	}

	vertices := make([]model.Vector3, vertexCount)
	for i := range vertices {
		x := r.readF32()
		y := r.readF32()
		z := r.readF32()
		_ = r.readU32() // padding
		vertices[i] = model.Vector3{X: x, Y: y, Z: z}
	}

	normals := make([]model.Vector3, normalCount)
	for i := range normals {
		x := r.readF32()
		y := r.readF32()
		z := r.readF32()
		normals[i] = model.Vector3{X: x, Y: y, Z: z}
	}
	if r.err != nil {
		return model.LOD{}, r.err
	}

	faces := make([]model.Face, faceCount)
	for i := range faces {
		face, err := parseFace(r)
		if err != nil {
			return model.LOD{}, fmt.Errorf("face %d: %w", i, err)
		}
		faces[i] = face
	}

	r.expectMagic("TAGG")
	if r.err != nil {
		return model.LOD{}, fmt.Errorf("expected TAGG section: %w", r.err)
	}

	taggs, err := parseTaggs(r, faces, int(vertexCount), int(faceCount))
	if err != nil {
		return model.LOD{}, err
	}

	resolution := r.readF32()
	if r.err != nil {
		return model.LOD{}, r.err
	}

	return model.LOD{
		Resolution:    resolution,
		Vertices:      vertices,
		Normals:       normals,
		Faces:         faces,
		Properties:    taggs.properties,
		PropertyOrder: taggs.propertyOrder,
		Selections:    taggs.selections,
		PointMasses:   taggs.pointMasses,
		UVSets:        taggs.uvSets,
	}, nil
}

// reconstructUVSet converts face-vertex-ordered UV pairs from a #UVSet# TAGG
// into a per-vertex slice. First-seen value wins at UV seams.
func reconstructUVSet(data []byte, faces []model.Face, vertexCount int) []model.UV {
	uvSet := make([]model.UV, vertexCount)
	seen := make([]bool, vertexCount)
	pos := 0
	for _, face := range faces {
		for _, vi := range face.Indices {
			if pos+8 > len(data) {
				return uvSet
			}
			u := math.Float32frombits(binary.LittleEndian.Uint32(data[pos:]))
			v := math.Float32frombits(binary.LittleEndian.Uint32(data[pos+4:]))
			pos += 8
			if int(vi) < vertexCount && !seen[vi] {
				uvSet[vi] = model.UV{U: u, V: v}
				seen[vi] = true
			}
		}
	}
	return uvSet
}

func parseFace(r *mlodReader) (model.Face, error) {
	vertexCount := r.readU32()
	if r.err != nil {
		return model.Face{}, r.err
	}
	if vertexCount != 3 && vertexCount != 4 {
		return model.Face{}, fmt.Errorf("unsupported face vertex count %d", vertexCount)
	}

	var indices []uint32
	var normalIndices []uint32
	var uvs []model.UV

	// always 4 slots in the binary, unused slots are zero-padded
	for i := uint32(0); i < 4; i++ {
		pointIndex := r.readU32()
		normalIndex := r.readU32()
		u := r.readF32()
		v := r.readF32()
		if i < vertexCount {
			indices = append(indices, pointIndex)
			normalIndices = append(normalIndices, normalIndex)
			uvs = append(uvs, model.UV{U: u, V: v})
		}
	}

	flags := r.readU32()
	texture := r.readString()
	material := r.readString()
	if r.err != nil {
		return model.Face{}, r.err
	}

	return model.Face{
		Indices:       indices,
		NormalIndices: normalIndices,
		UVs:           uvs,
		Flags:         flags,
		Texture:       texture,
		Material:      material,
	}, nil
}

type taggResult struct {
	properties    map[string]string
	propertyOrder []string
	selections    []model.Selection
	pointMasses   []float32
	uvSets        [][]model.UV
}

func parseTaggs(r *mlodReader, faces []model.Face, vertexCount, faceCount int) (taggResult, error) {
	result := taggResult{
		properties: make(map[string]string),
	}

	for {
		_ = r.readU8() // flag, always 1
		name := r.readString()
		dataLen := r.readU32()
		if r.err != nil {
			return result, r.err
		}

		data := r.readBytes(int(dataLen))
		if r.err != nil {
			return result, r.err
		}

		switch name {
		case "#EndOfFile#":
			return result, nil

		case "#Mass#":
			masses := make([]float32, vertexCount)
			for i := range masses {
				if i*4+4 > len(data) {
					break
				}
				masses[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
			}
			result.pointMasses = masses

		case "#UVSet#":
			if len(data) < 4 {
				break
			}
			id := binary.LittleEndian.Uint32(data[0:4])
			if id == 0 {
				break // redundant with Face.UVs; primary UV comes from face records
			}
			uvSet := reconstructUVSet(data[4:], faces, vertexCount)
			for len(result.uvSets) <= int(id) {
				result.uvSets = append(result.uvSets, nil)
			}
			result.uvSets[id] = uvSet

		case "#Property#":
			if len(data) >= 128 {
				key := nullTermString(data[:64])
				value := nullTermString(data[64:128])
				if key != "" {
					result.properties[key] = value
					result.propertyOrder = append(result.propertyOrder, key)
				}
			}

		default:
			// named selection
			sel := parseSelection(name, data, vertexCount, faceCount)
			result.selections = append(result.selections, sel)
		}
	}
}

func parseSelection(name string, data []byte, vertexCount, faceCount int) model.Selection {
	sel := model.Selection{Name: name}

	for vi := 0; vi < vertexCount && vi < len(data); vi++ {
		w := data[vi]
		if w != 0 {
			sel.VertexIndices = append(sel.VertexIndices, uint32(vi))
			sel.VertexWeights = append(sel.VertexWeights, w)
		}
	}

	for fi := 0; fi < faceCount; fi++ {
		idx := vertexCount + fi
		if idx < len(data) && data[idx] != 0 {
			sel.FaceIndices = append(sel.FaceIndices, uint32(fi))
		}
	}

	return sel
}

func nullTermString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// mlodReader is a sequential binary reader that accumulates the first error.
type mlodReader struct {
	data []byte
	pos  int
	err  error
}

func (r *mlodReader) readU8() uint8 {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.data) {
		r.err = fmt.Errorf("unexpected EOF at offset %d", r.pos)
		return 0
	}
	v := r.data[r.pos]
	r.pos++
	return v
}

func (r *mlodReader) readU32() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos+4 > len(r.data) {
		r.err = fmt.Errorf("unexpected EOF at offset %d: need 4 bytes, have %d", r.pos, len(r.data)-r.pos)
		return 0
	}
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v
}

func (r *mlodReader) readF32() float32 {
	return math.Float32frombits(r.readU32())
}

func (r *mlodReader) readBytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if r.pos+n > len(r.data) {
		r.err = fmt.Errorf("unexpected EOF at offset %d: need %d bytes, have %d", r.pos, n, len(r.data)-r.pos)
		return nil
	}
	out := make([]byte, n)
	copy(out, r.data[r.pos:r.pos+n])
	r.pos += n
	return out
}

func (r *mlodReader) readString() string {
	if r.err != nil {
		return ""
	}
	start := r.pos
	for r.pos < len(r.data) {
		if r.data[r.pos] == 0 {
			s := string(r.data[start:r.pos])
			r.pos++ // consume null terminator
			return s
		}
		r.pos++
	}
	r.err = fmt.Errorf("unterminated string at offset %d", start)
	return ""
}

func (r *mlodReader) expectMagic(magic string) {
	if r.err != nil {
		return
	}
	b := r.readBytes(len(magic))
	if r.err != nil {
		return
	}
	if string(b) != magic {
		r.err = fmt.Errorf("expected %q at offset %d, got %q", magic, r.pos-len(magic), b)
	}
}
