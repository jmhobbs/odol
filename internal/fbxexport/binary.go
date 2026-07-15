package fbxexport

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// fbxBinaryVersion matches the version used by canonical DayZ FBX SDK
// exports (samples/fbx/*.fbx) - see plan_fbx-parity.md. Being >= 7500 means
// node records use the 8-byte (not 4-byte) offset/length layout and a
// 25-byte (not 13-byte) NULL-record; see binWriter.wide.
const fbxBinaryVersion = 7700

// binWriter accumulates a binary FBX document in memory. Node records need
// their EndOffset and PropertyListLen patched in after their contents are
// known, so writes go through a plain growable byte slice (indexed
// patching stays valid across reallocation) rather than an io.Writer.
type binWriter struct {
	buf  []byte
	wide bool // node headers use 8-byte fields; NULL-records are 25 bytes (FBX version >= 7500)
}

func (bw *binWriter) len() int { return len(bw.buf) }

func (bw *binWriter) writeBytes(b []byte) { bw.buf = append(bw.buf, b...) }

func (bw *binWriter) writeUint8(v uint8) { bw.buf = append(bw.buf, v) }

func (bw *binWriter) writeUint32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	bw.writeBytes(b[:])
}

func (bw *binWriter) writeInt32(v int32) { bw.writeUint32(uint32(v)) }

func (bw *binWriter) writeUint64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	bw.writeBytes(b[:])
}

func (bw *binWriter) writeInt64(v int64) { bw.writeUint64(uint64(v)) }

func (bw *binWriter) writeFloat64(v float64) { bw.writeUint64(math.Float64bits(v)) }

func (bw *binWriter) writeString(s string) {
	bw.writeUint32(uint32(len(s)))
	bw.writeBytes([]byte(s))
}

// patchUint32 overwrites a uint32 reserved earlier at offset. offset indexes
// into the current bw.buf, so this stays correct regardless of any slice
// growth/reallocation that happened since offset was recorded.
func (bw *binWriter) patchUint32(offset int, v uint32) {
	binary.LittleEndian.PutUint32(bw.buf[offset:offset+4], v)
}

// patchUint64 is patchUint32's 8-byte counterpart, used for wide node headers.
func (bw *binWriter) patchUint64(offset int, v uint64) {
	binary.LittleEndian.PutUint64(bw.buf[offset:offset+8], v)
}

// writeNodeHeaderField writes one of a node record's three header fields
// (EndOffset, NumProperties, PropertyListLen), 4 or 8 bytes depending on
// bw.wide, and returns the byte offset it was written at (for patching).
func (bw *binWriter) writeNodeHeaderField(v uint64) (offset int) {
	offset = bw.len()
	if bw.wide {
		bw.writeUint64(v)
	} else {
		bw.writeUint32(uint32(v))
	}
	return offset
}

// patchNodeHeaderField patches a field previously written by
// writeNodeHeaderField, at the matching width.
func (bw *binWriter) patchNodeHeaderField(offset int, v uint64) {
	if bw.wide {
		bw.patchUint64(offset, v)
	} else {
		bw.patchUint32(offset, uint32(v))
	}
}

// writeBinary builds the generic FBX node tree for s and renders it as
// binary FBX 7.4.
func writeBinary(w io.Writer, s *meshScene) error {
	nodes := buildDocumentNodes(s, time.Now())
	return writeBinaryNodes(w, nodes)
}

// writeBinaryNodes renders a pre-built document tree (top-level siblings,
// as returned by buildDocumentNodes) as binary FBX 7.4, including the
// header and footer. The footer's timestamp-derived code is read back out
// of the FBXHeaderExtension/CreationTimeStamp node already present in
// nodes, so there is exactly one timestamp for the whole file.
func writeBinaryNodes(w io.Writer, nodes []*node) error {
	bw := &binWriter{wide: fbxBinaryVersion >= 7500}
	writeBinaryHeader(bw, fbxBinaryVersion)
	for _, n := range nodes {
		writeBinaryNode(bw, n)
	}
	writeBinaryNullRecord(bw)
	writeBinaryFooter(bw, extractCreationTimestamp(nodes), fbxBinaryVersion)

	_, err := w.Write(bw.buf)
	return err
}

func writeBinaryHeader(bw *binWriter, version uint32) {
	bw.writeBytes([]byte("Kaydara FBX Binary  \x00"))
	bw.writeBytes([]byte{0x1A, 0x00})
	bw.writeUint32(version)
}

// writeBinaryNode writes n as a binary node record, recursing into children
// and patching EndOffset/PropertyListLen once the record's true size is
// known. A NULL-record terminator is written whenever n.isList(), even if
// it has zero children - this is what lets a decoder tell "leaf" and
// "empty list" apart, matching the ASCII renderer's brace behavior.
func writeBinaryNode(bw *binWriter, n *node) {
	endOffsetPos := bw.writeNodeHeaderField(0)
	bw.writeNodeHeaderField(uint64(len(n.props)))
	propLenPos := bw.writeNodeHeaderField(0)
	bw.writeUint8(uint8(len(n.name)))
	bw.writeBytes([]byte(n.name))

	propStart := bw.len()
	for _, prop := range n.props {
		writeBinaryProperty(bw, prop)
	}
	bw.patchNodeHeaderField(propLenPos, uint64(bw.len()-propStart))

	if n.isList() {
		for _, c := range n.children {
			writeBinaryNode(bw, c)
		}
		writeBinaryNullRecord(bw)
	}

	bw.patchNodeHeaderField(endOffsetPos, uint64(bw.len()))
}

// writeBinaryNullRecord terminates a nested node list: 13 zero bytes
// (4+4+4+1) at the narrow width, 25 (8+8+8+1) at the wide width.
func writeBinaryNullRecord(bw *binWriter) {
	if bw.wide {
		bw.writeBytes(make([]byte, 25))
	} else {
		bw.writeBytes(make([]byte, 13))
	}
}

// writeBinaryProperty encodes a single node property. v must be one of the
// concrete types node.props documents; anything else is a programmer error
// in a tree builder, not a runtime input error, so it panics.
func writeBinaryProperty(bw *binWriter, v any) {
	switch t := v.(type) {
	case string:
		bw.writeUint8('S')
		bw.writeString(binaryFBXName(t))
	case []byte:
		bw.writeUint8('R')
		bw.writeUint32(uint32(len(t)))
		bw.writeBytes(t)
	case int32:
		bw.writeUint8('I')
		bw.writeInt32(t)
	case int64:
		bw.writeUint8('L')
		bw.writeInt64(t)
	case float64:
		bw.writeUint8('D')
		bw.writeFloat64(t)
	case shadingFlag:
		bw.writeUint8('C')
		if t {
			bw.writeUint8(1)
		} else {
			bw.writeUint8(0)
		}
	case []int32:
		raw := make([]byte, len(t)*4)
		for i, x := range t {
			binary.LittleEndian.PutUint32(raw[i*4:], uint32(x))
		}
		writeBinaryArrayProperty(bw, 'i', len(t), raw)
	case []float64:
		raw := make([]byte, len(t)*8)
		for i, x := range t {
			binary.LittleEndian.PutUint64(raw[i*8:], math.Float64bits(x))
		}
		writeBinaryArrayProperty(bw, 'd', len(t), raw)
	default:
		panic(fmt.Sprintf("fbxexport: unsupported binary property type %T", v))
	}
}

// writeBinaryArrayProperty writes an array-typed property uncompressed
// (Encoding=0). Canonical DayZ FBX exports compress arrays unconditionally
// (Encoding=1, zlib), which this deliberately does not match - see
// plan_fbx-parity.md. Blender's newer, still-experimental native FBX
// importer bundles ufbx 0.20.0, which has a confirmed checksum-verification
// bug (not a data-correctness one - independently verified our zlib streams
// are spec-correct) that rejects some, though not all, valid zlib-compressed
// arrays with "Bad DEFLATE data". No compression level or block-type choice
// reliably avoids it. Writing arrays uncompressed sidesteps DEFLATE/zlib
// entirely, guaranteeing compatibility with every FBX reader at the cost of
// larger files and this one point of divergence from canonical's structure.
//
// TODO: re-enable zlib compression once Blender ships a ufbx build past the
// fix (upstream commits e06d9f28/23e0d7ec, already merged). See
// plan_fbx-compression-revert.md for the re-enablement plan.
func writeBinaryArrayProperty(bw *binWriter, typeCode byte, count int, raw []byte) {
	bw.writeUint8(typeCode)
	bw.writeUint32(uint32(count))
	bw.writeUint32(0) // encoding: uncompressed
	bw.writeUint32(uint32(len(raw)))
	bw.writeBytes(raw)
}

// binaryFBXName re-encodes an ASCII-style "Class::Name" object identifier
// (e.g. "Geometry::55galdrum") for binary FBX. Binary readers (confirmed
// against Blender's io_scene_fbx importer) split an object's name/class
// string property on "\x00\x01" with the tokens in reverse order, not on
// "::" - so "Geometry::55galdrum" must be written as "55galdrum\x00\x01Geometry".
// This mirrors FbxBinaryWriter.WriteString in github.com/hamish-milne/FbxWriter
// and applies to any string property containing "::", matching real FBX
// binary files rather than just this exporter's own name/class properties.
func binaryFBXName(s string) string {
	const asciiSep = "::"
	if !strings.Contains(s, asciiSep) {
		return s
	}
	tokens := strings.Split(s, asciiSep)
	reversed := make([]string, len(tokens))
	for i, tok := range tokens {
		reversed[len(tokens)-1-i] = tok
	}
	return strings.Join(reversed, "\x00\x01")
}

// creationTimestamp is the subset of FBXHeaderExtension/CreationTimeStamp
// needed to derive the binary footer code.
type creationTimestamp struct {
	year, month, day, hour, minute, second, millisecond int32
}

func extractCreationTimestamp(nodes []*node) creationTimestamp {
	hdr := findTopNode(nodes, "FBXHeaderExtension")
	ts := findChildNode(hdr, "CreationTimeStamp")
	return creationTimestamp{
		year:        int32Prop(ts, "Year"),
		month:       int32Prop(ts, "Month"),
		day:         int32Prop(ts, "Day"),
		hour:        int32Prop(ts, "Hour"),
		minute:      int32Prop(ts, "Minute"),
		second:      int32Prop(ts, "Second"),
		millisecond: int32Prop(ts, "Millisecond"),
	}
}

func findTopNode(nodes []*node, name string) *node {
	for _, n := range nodes {
		if n.name == name {
			return n
		}
	}
	panic(fmt.Sprintf("fbxexport: missing top-level node %q", name))
}

func findChildNode(n *node, name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	panic(fmt.Sprintf("fbxexport: node %q missing child %q", n.name, name))
}

func int32Prop(n *node, name string) int32 {
	c := findChildNode(n, name)
	return c.props[0].(int32)
}

// Footer layout, reverse-engineered from github.com/hamish-milne/FbxWriter
// (Fbx/FbxBinary.cs): a 16-byte code derived from the creation timestamp,
// 20 zero bytes, the version as int32, 120 zero bytes, then a fixed 16-byte
// magic trailer. No known open-source reader validates the footer code
// itself; it's included for compatibility with Autodesk tooling.
var (
	footerSourceID = []byte{0x58, 0xAB, 0xA9, 0xF0, 0x6C, 0xA2, 0xD8, 0x3F, 0x4D, 0x47, 0x49, 0xA3, 0xB4, 0xB2, 0xE7, 0x3D}
	footerKey      = []byte{0xE2, 0x4F, 0x7B, 0x5F, 0xCD, 0xE4, 0xC8, 0x6D, 0xDB, 0xD8, 0xFB, 0xD7, 0x40, 0x58, 0xC6, 0x78}
	footerMagic    = []byte{0xF8, 0x5A, 0x8C, 0x6A, 0xDE, 0xF5, 0xD9, 0x7E, 0xEC, 0xE9, 0x0C, 0xE3, 0x75, 0x8F, 0x29, 0x0B}
)

func footerEncrypt(a, b []byte) []byte {
	out := append([]byte(nil), a...)
	c := byte(64)
	for i := range out {
		out[i] = out[i] ^ (c ^ b[i])
		c = out[i]
	}
	return out
}

func generateFooterCode(ts creationTimestamp) []byte {
	mangled := []byte(fmt.Sprintf("%02d%02d%02d%02d%02d%04d%02d",
		ts.second, ts.month, ts.hour, ts.day, ts.millisecond/10, ts.year, ts.minute))

	code := footerEncrypt(footerSourceID, mangled)
	code = footerEncrypt(code, footerKey)
	code = footerEncrypt(code, mangled)
	return code
}

func writeBinaryFooter(bw *binWriter, ts creationTimestamp, version uint32) {
	bw.writeBytes(generateFooterCode(ts))
	bw.writeBytes(make([]byte, 20))
	bw.writeUint32(version)
	bw.writeBytes(make([]byte, 120))
	bw.writeBytes(footerMagic)
}
