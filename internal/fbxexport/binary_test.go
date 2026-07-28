package fbxexport

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test-only binary FBX decoder ---
//
// Decodes what this writer emits, plus what's needed to read arbitrary
// external binary FBX files (samples/fbx/*.fbx, exported by real DayZ
// tooling) for parity comparison in parity_test.go: node records in either
// the pre-7500 4-byte or the >=7500 8-byte header layout, primitive
// properties (S, I, L, D, C, R), and array properties (i, d) whether
// zlib-compressed (Encoding=1) or not (Encoding=0). Nothing else - this is
// not a general-purpose reader.

type binReader struct {
	buf  []byte
	pos  int
	wide bool // node headers use 8-byte fields (FBX version >= 7500)

	// arrayEncodings collects the Encoding value of every array property
	// read, in encounter order - side-channel used by parity tests to check
	// compression usage without changing decodedNode's value-only shape.
	arrayEncodings []uint32
}

// decodeDocument reads a full binary FBX file: the 27-byte header (which
// determines node-record width for the rest of the file), the top-level
// node list, and returns whatever bytes trail the top-level NULL-record as
// the footer, plus the Encoding value of every array property encountered.
func decodeDocument(t *testing.T, data []byte) (version uint32, top []*decodedNode, footer []byte, arrayEncodings []uint32) {
	t.Helper()
	require.True(t, bytes.HasPrefix(data, []byte("Kaydara FBX Binary  \x00")), "binary FBX magic header")
	version = binary.LittleEndian.Uint32(data[23:27])
	br := &binReader{buf: data, pos: 27, wide: version >= 7500}
	for {
		n := br.readNode(t)
		if n == nil {
			break
		}
		top = append(top, n)
	}
	return version, top, data[br.pos:], br.arrayEncodings
}

func (br *binReader) readUint8() uint8 {
	v := br.buf[br.pos]
	br.pos++
	return v
}

func (br *binReader) readBytes(n int) []byte {
	b := br.buf[br.pos : br.pos+n]
	br.pos += n
	return b
}

func (br *binReader) readUint32() uint32 {
	v := binary.LittleEndian.Uint32(br.buf[br.pos : br.pos+4])
	br.pos += 4
	return v
}

func (br *binReader) readUint64() uint64 {
	v := binary.LittleEndian.Uint64(br.buf[br.pos : br.pos+8])
	br.pos += 8
	return v
}

type decodedNode struct {
	name      string
	props     []any
	children  []*decodedNode
	endOffset uint32
}

func (dn *decodedNode) child(name string) *decodedNode {
	for _, c := range dn.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (dn *decodedNode) childrenNamed(name string) []*decodedNode {
	var out []*decodedNode
	for _, c := range dn.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// readNode reads one node record, or returns nil if it's a NULL-record.
func (br *binReader) readNode(t *testing.T) *decodedNode {
	t.Helper()
	var endOffset, numProps, propListLen uint64
	if br.wide {
		endOffset, numProps, propListLen = br.readUint64(), br.readUint64(), br.readUint64()
	} else {
		endOffset, numProps, propListLen = uint64(br.readUint32()), uint64(br.readUint32()), uint64(br.readUint32())
	}
	nameLen := br.readUint8()
	if endOffset == 0 && numProps == 0 && propListLen == 0 && nameLen == 0 {
		return nil
	}
	name := string(br.readBytes(int(nameLen)))

	propStart := br.pos
	props := make([]any, 0, numProps)
	for i := uint64(0); i < numProps; i++ {
		props = append(props, br.readProperty(t))
	}
	require.Equal(t, propListLen, uint64(br.pos-propStart), "node %q property list length", name)

	var children []*decodedNode
	for uint64(br.pos) < endOffset {
		c := br.readNode(t)
		if c == nil {
			break
		}
		children = append(children, c)
	}
	require.Equal(t, endOffset, uint64(br.pos), "node %q end offset", name)

	return &decodedNode{name: name, props: props, children: children, endOffset: uint32(endOffset)}
}

func (br *binReader) readProperty(t *testing.T) any {
	t.Helper()
	code := br.readUint8()
	switch code {
	case 'S':
		n := br.readUint32()
		return decodeBinaryFBXName(string(br.readBytes(int(n))))
	case 'I':
		return int32(br.readUint32())
	case 'L':
		return int64(br.readUint64())
	case 'D':
		return math.Float64frombits(br.readUint64())
	case 'C':
		return br.readUint8() != 0
	case 'R':
		n := br.readUint32()
		return br.readBytes(int(n))
	case 'i':
		return br.readInt32Array(t)
	case 'd':
		return br.readFloat64Array(t)
	default:
		t.Fatalf("test decoder: unsupported property type code %q", code)
		return nil
	}
}

// decodeBinaryFBXName reverses binaryFBXName's "\x00\x01"-joined,
// reverse-order encoding of "Class::Name" identifiers back to the natural
// "Class::Name" form, so tests can assert against the same strings used
// when building the node tree.
func decodeBinaryFBXName(s string) string {
	const binSep = "\x00\x01"
	if !strings.Contains(s, binSep) {
		return s
	}
	tokens := strings.Split(s, binSep)
	reversed := make([]string, len(tokens))
	for i, tok := range tokens {
		reversed[len(tokens)-1-i] = tok
	}
	return strings.Join(reversed, "::")
}

// readArrayPayload reads the array-property header (length, encoding,
// compressed length) and returns the decompressed element bytes, handling
// both Encoding=0 (raw) and Encoding=1 (zlib-compressed) - real FBX files
// (including canonical DayZ exports) always use Encoding=1.
func (br *binReader) readArrayPayload(t *testing.T, elemSize int) (length uint32, raw []byte) {
	t.Helper()
	length = br.readUint32()
	encoding := br.readUint32()
	compLen := br.readUint32()
	data := br.readBytes(int(compLen))
	br.arrayEncodings = append(br.arrayEncodings, encoding)
	switch encoding {
	case 0:
		require.Equal(t, int(length)*elemSize, len(data), "uncompressed array payload length")
		return length, data
	case 1:
		zr, err := zlib.NewReader(bytes.NewReader(data))
		require.NoError(t, err, "zlib reader init")
		raw, err = io.ReadAll(zr)
		require.NoError(t, err, "zlib decompress")
		require.Equal(t, int(length)*elemSize, len(raw), "decompressed array payload length")
		return length, raw
	default:
		t.Fatalf("test decoder: unsupported array encoding %d", encoding)
		return 0, nil
	}
}

func (br *binReader) readInt32Array(t *testing.T) []int32 {
	t.Helper()
	length, data := br.readArrayPayload(t, 4)
	out := make([]int32, length)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return out
}

func (br *binReader) readFloat64Array(t *testing.T) []float64 {
	t.Helper()
	length, data := br.readArrayPayload(t, 8)
	out := make([]float64, length)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[i*8:]))
	}
	return out
}

// --- tests ---

func TestWriteBinaryHeaderBytes(t *testing.T) {
	bw := &binWriter{}
	writeBinaryHeader(bw, fbxBinaryVersion)

	require.Len(t, bw.buf, 27)
	assert.Equal(t, "Kaydara FBX Binary  \x00", string(bw.buf[0:21]))
	assert.Equal(t, []byte{0x1A, 0x00}, bw.buf[21:23])
	assert.Equal(t, uint32(fbxBinaryVersion), binary.LittleEndian.Uint32(bw.buf[23:27]))
}

func TestWriteBinaryNodeLeafRoundTrip(t *testing.T) {
	bw := &binWriter{}
	writeBinaryNode(bw, leaf("Count", int32(5)))

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.NotNil(t, got)
	assert.Equal(t, "Count", got.name)
	assert.Equal(t, []any{int32(5)}, got.props)
	assert.Nil(t, got.children)
	assert.Equal(t, uint32(len(bw.buf)), got.endOffset)
	assert.Equal(t, len(bw.buf), br.pos, "reader should consume exactly the written bytes")
}

func TestWriteBinaryNodeScalarPropertyTypesRoundTrip(t *testing.T) {
	n := &node{
		name:  "P",
		props: []any{"ODOL_SourceFamily", "KString", "", "", "ODOL", int64(7), 1.5, shadingFlag(true)},
	}
	bw := &binWriter{}
	writeBinaryNode(bw, n)

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.NotNil(t, got)
	assert.Equal(t, []any{"ODOL_SourceFamily", "KString", "", "", "ODOL", int64(7), 1.5, true}, got.props)
}

func TestWriteBinaryArrayPropertyRoundTrip(t *testing.T) {
	values := []float64{0, 0, 0, 1, 0, 0, 0, 1, 0}
	bw := &binWriter{}
	writeBinaryNode(bw, leaf("Vertices", values))

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.NotNil(t, got)
	require.Len(t, got.props, 1)
	assert.Equal(t, values, got.props[0])
}

func TestWriteBinaryInt32ArrayPropertyRoundTrip(t *testing.T) {
	values := []int32{0, 1, -3}
	bw := &binWriter{}
	writeBinaryNode(bw, leaf("PolygonVertexIndex", values))

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.Len(t, got.props, 1)
	assert.Equal(t, values, got.props[0])
}

// Arrays are written uncompressed (Encoding=0) rather than zlib-compressed,
// despite canonical DayZ FBX exports always compressing - see the TODO on
// writeBinaryArrayProperty and plan_fbx-compression-revert.md for why.
func TestWriteBinaryArraysAreUncompressed(t *testing.T) {
	bw := &binWriter{}
	writeBinaryNode(bw, leaf("Vertices", []float64{0, 0, 0, 1, 0, 0, 0, 1, 0}))

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.NotNil(t, got)
	require.Len(t, br.arrayEncodings, 1)
	assert.Equal(t, uint32(0), br.arrayEncodings[0], "array should be uncompressed (Encoding=0)")
}

func TestWriteBinaryEmptyArrayDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		bw := &binWriter{}
		writeBinaryNode(bw, leaf("Vertices", []float64{}))

		br := &binReader{buf: bw.buf}
		got := br.readNode(t)
		require.NotNil(t, got)
		assert.Equal(t, []float64{}, got.props[0])
	})
}

func TestWriteBinaryEmptyListGetsNullRecordLeafDoesNot(t *testing.T) {
	leafNode := leaf("Culling", "CullingOff")
	listNode := &node{name: "References", children: []*node{}}

	bwLeaf := &binWriter{}
	writeBinaryNode(bwLeaf, leafNode)
	bwList := &binWriter{}
	writeBinaryNode(bwList, listNode)

	// Same-shaped header + a single string prop of very different length,
	// so compare the *presence* of the extra 13-byte NULL-record via
	// decoding, not raw byte-length arithmetic against each other.
	brLeaf := &binReader{buf: bwLeaf.buf}
	gotLeaf := brLeaf.readNode(t)
	assert.Nil(t, gotLeaf.children)
	assert.Equal(t, len(bwLeaf.buf), brLeaf.pos)

	brList := &binReader{buf: bwList.buf}
	gotList := brList.readNode(t)
	assert.Nil(t, gotList.children) // zero real children, but...
	assert.Equal(t, len(bwList.buf), brList.pos)

	// The list node's on-disk size must include the 13-byte NULL-record
	// even though it has no real children: header(13) + name(9) + 0 props
	// + NULL-record(13) = 35.
	assert.Equal(t, 13+len("References")+13, len(bwList.buf))
	// The leaf's size is just header + name + one string property, no
	// NULL-record: header(13) + name(7) + prop(1 type byte + 4 len + 10 data).
	assert.Equal(t, 13+len("Culling")+1+4+len("CullingOff"), len(bwLeaf.buf))
}

func TestWriteBinaryWideNodeRoundTrip(t *testing.T) {
	bw := &binWriter{wide: true}
	writeBinaryNode(bw, leaf("Count", int32(5)))

	br := &binReader{buf: bw.buf, wide: true}
	got := br.readNode(t)
	require.NotNil(t, got)
	assert.Equal(t, "Count", got.name)
	assert.Equal(t, []any{int32(5)}, got.props)
	assert.Equal(t, len(bw.buf), br.pos)
	// Wide header is 8+8+8+1=25 bytes vs narrow's 13.
	assert.Equal(t, 25+len("Count")+1+4, len(bw.buf)) // +1 type byte, +4 int32 value
}

func TestWriteBinaryWideEmptyListGetsA25ByteNullRecord(t *testing.T) {
	listNode := &node{name: "References", children: []*node{}}

	bw := &binWriter{wide: true}
	writeBinaryNode(bw, listNode)

	br := &binReader{buf: bw.buf, wide: true}
	got := br.readNode(t)
	assert.Nil(t, got.children)
	assert.Equal(t, len(bw.buf), br.pos)

	// header(25) + name(10) + 0 props + NULL-record(25) = 60.
	assert.Equal(t, 25+len("References")+25, len(bw.buf))
}

func TestWriteBinaryNestedThreeLevelsRoundTrip(t *testing.T) {
	part := &meshPart{name: "object_NNN1", geomID: 1, modelID: 2}
	s := &meshScene{
		parts:         []meshPart{*part},
		sourceFamily:  "ODOL",
		sourceVersion: 7,
	}
	model := buildMeshModelNode(s, part) // Model -> Properties70 -> P (3 levels)

	bw := &binWriter{}
	writeBinaryNode(bw, model)

	br := &binReader{buf: bw.buf}
	got := br.readNode(t)
	require.NotNil(t, got)
	assert.Equal(t, "Model", got.name)

	props70 := got.child("Properties70")
	require.NotNil(t, props70)
	require.NotEmpty(t, props70.children)

	var found bool
	for _, p := range props70.children {
		if p.name == "P" && len(p.props) > 0 && p.props[0] == "ODOL_SourceFamily" {
			found = true
			assert.Equal(t, []any{"ODOL_SourceFamily", "KString", "", "", "ODOL"}, p.props)
		}
	}
	assert.True(t, found, "expected an ODOL_SourceFamily P node")
	assert.Equal(t, len(bw.buf), br.pos)
}

func TestGenerateFooterCodeMatchesIndependentlyComputedValue(t *testing.T) {
	// Cross-checked against an independent Python re-implementation of the
	// XOR algorithm from FbxBinary.cs (not derived from this Go code).
	ts := creationTimestamp{year: 2026, month: 7, day: 13, hour: 11, minute: 30, second: 45, millisecond: 250}
	want, err := hex.DecodeString("fabcaf08d5ced666b473f9841ff8287a")
	require.NoError(t, err)
	assert.Equal(t, want, generateFooterCode(ts))
}

func TestGenerateFooterCodeVariesWithTimestamp(t *testing.T) {
	a := generateFooterCode(creationTimestamp{year: 2026, month: 7, day: 13, hour: 11, minute: 30, second: 45, millisecond: 250})
	b := generateFooterCode(creationTimestamp{year: 2026, month: 7, day: 13, hour: 11, minute: 30, second: 46, millisecond: 250})
	assert.NotEqual(t, a, b)
}

func TestWriteBinaryFooterLayout(t *testing.T) {
	ts := creationTimestamp{year: 2026, month: 7, day: 13, hour: 11, minute: 30, second: 45, millisecond: 250}
	bw := &binWriter{}
	writeBinaryFooter(bw, ts, fbxBinaryVersion)

	require.Len(t, bw.buf, 176)
	assert.Equal(t, generateFooterCode(ts), bw.buf[0:16])
	assert.True(t, bytes.Equal(bw.buf[16:36], make([]byte, 20)), "20 zero bytes after footer code")
	assert.Equal(t, uint32(fbxBinaryVersion), binary.LittleEndian.Uint32(bw.buf[36:40]))
	assert.True(t, bytes.Equal(bw.buf[40:160], make([]byte, 120)), "120 zero bytes before magic trailer")
	assert.Equal(t, footerMagic, bw.buf[160:176])
}

func TestWriteBinaryNodesFullDocumentRoundTrip(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:    "object_NNN1",
			geomID:  1,
			modelID: 2,
			geometry: geometryData{
				vertices:  []float64{0, 0, 0, 1, 0, 0, 0, 1, 0},
				polyIndex: []int32{0, 1, -3},
				normals:   []float64{0, 0, 1, 0, 0, 1, 0, 0, 1},
				uvSets:    [][]float64{{0, 0, 1, 0, 0.5, 1}},
				matIndex:  []int32{0},
			},
			localMaterials: []int{0},
		}},
		sourceFamily:  "ODOL",
		sourceVersion: 7,
		lodResolution: 1.5,
		selections:    []selectionData{{id: 5, name: "cargo", vertexIndices: []uint32{0, 2}}},
		materials:     []materialData{{id: 3, name: "Mat_0", texture: "tex.paa", mat: "Mat"}},
		proxies:       []proxyData{{id: 4, name: "Proxy_0", path: "proxy.p3d"}},
	}
	fixedNow := time.Date(2026, 7, 13, 11, 30, 45, 250_000_000, time.UTC)
	nodes := buildDocumentNodes(s, fixedNow)

	var buf bytes.Buffer
	require.NoError(t, writeBinaryNodes(&buf, nodes))
	data := buf.Bytes()

	version, top, footer, _ := decodeDocument(t, data)
	assert.Equal(t, uint32(fbxBinaryVersion), version)

	var names []string
	for _, n := range top {
		names = append(names, n.name)
	}
	assert.Equal(t, []string{
		"FBXHeaderExtension", "FileId", "CreationTime", "Creator",
		"GlobalSettings", "Documents", "References", "Definitions", "Objects", "Connections", "Takes",
	}, names)

	var objects *decodedNode
	for _, n := range top {
		if n.name == "Objects" {
			objects = n
		}
	}
	require.NotNil(t, objects)
	geom := objects.child("Geometry")
	require.NotNil(t, geom)
	assert.Equal(t, []any{s.parts[0].geometry.vertices}, geom.child("Vertices").props)
	assert.Equal(t, []any{s.parts[0].geometry.polyIndex}, geom.child("PolygonVertexIndex").props)

	models := objects.childrenNamed("Model")
	require.Len(t, models, 3) // mesh part + proxy + selection
	assert.Equal(t, []any{int64(2), "Model::object_NNN1", "Mesh"}, models[0].props)

	// Whatever trails the top-level NULL-record is the footer.
	require.Len(t, footer, 176)
	assert.Equal(t, footerMagic, footer[160:176])
}
