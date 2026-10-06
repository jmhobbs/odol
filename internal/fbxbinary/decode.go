// Package fbxbinary decodes binary FBX documents (the "Kaydara FBX Binary"
// container format) into a generic node tree. It mirrors the structure
// internal/fbxexport's binary writer produces (see docs/fbx_binary_specification.md
// and docs/fbx-binary-format.txt) but reads in the opposite direction, so
// FBX files produced by external tools (DCC exporters, or this repo's own
// writer) can be walked without a full FBX SDK.
//
// Only the binary container is handled - ASCII FBX is out of scope.
package fbxbinary

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

const magic = "Kaydara FBX Binary  \x00"

// headerSize is the fixed 27-byte binary FBX header: the 21-byte magic,
// 1 unknown byte, 1 endianness byte, and a 4-byte little-endian version.
const headerSize = 27

// wideVersionThreshold is the FBX version at which node records switch from
// 4-byte to 8-byte EndOffset/NumProperties/PropertyListLen fields.
const wideVersionThreshold = 7500

// Node is one binary FBX node record: a name, an ordered tuple of typed
// properties, and an optional nested list of children.
//
// Property values decode to one of: string, bool, int16, int32, int64,
// float32, float64, []byte, []int32, []int64, []float32, []float64, []bool.
type Node struct {
	Name     string
	Props    []any
	Children []*Node
}

// Child returns the first direct child named name, or nil if there is none.
func (n *Node) Child(name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ChildrenNamed returns every direct child named name, in document order.
func (n *Node) ChildrenNamed(name string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// Document is a fully decoded binary FBX file.
type Document struct {
	Version uint32
	Nodes   []*Node // top-level siblings
}

// Top returns the first top-level node named name, or nil if there is none.
func (d *Document) Top(name string) *Node {
	for _, n := range d.Nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// Decode parses a complete binary FBX document.
func Decode(data []byte) (*Document, error) {
	if !bytes.HasPrefix(data, []byte(magic)) {
		return nil, fmt.Errorf("fbxbinary: not a binary FBX file (missing %q magic header)", magic)
	}
	if len(data) < headerSize {
		return nil, fmt.Errorf("fbxbinary: truncated header (%d bytes, need %d)", len(data), headerSize)
	}

	version := binary.LittleEndian.Uint32(data[23:27])
	d := &decoder{buf: data, pos: headerSize, wide: version >= wideVersionThreshold}

	var nodes []*Node
	for {
		n, err := d.readNode()
		if err != nil {
			return nil, err
		}
		if n == nil {
			break
		}
		nodes = append(nodes, n)
	}

	return &Document{Version: version, Nodes: nodes}, nil
}

type decoder struct {
	buf  []byte
	pos  int
	wide bool
}

func (d *decoder) need(n int) error {
	if d.pos+n > len(d.buf) {
		return fmt.Errorf("fbxbinary: unexpected end of file at offset %d (need %d more bytes)", d.pos, n)
	}
	return nil
}

func (d *decoder) readUint8() (uint8, error) {
	if err := d.need(1); err != nil {
		return 0, err
	}
	v := d.buf[d.pos]
	d.pos++
	return v, nil
}

func (d *decoder) readBytes(n int) ([]byte, error) {
	if err := d.need(n); err != nil {
		return nil, err
	}
	b := d.buf[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

func (d *decoder) readUint32() (uint32, error) {
	b, err := d.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (d *decoder) readUint64() (uint64, error) {
	b, err := d.readBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// readNode reads one node record, or returns (nil, nil) at a NULL-record
// (the all-zero terminator that closes a nested node list).
func (d *decoder) readNode() (*Node, error) {
	var endOffset, numProps, propListLen uint64
	var err error
	if d.wide {
		if endOffset, err = d.readUint64(); err != nil {
			return nil, err
		}
		if numProps, err = d.readUint64(); err != nil {
			return nil, err
		}
		if propListLen, err = d.readUint64(); err != nil {
			return nil, err
		}
	} else {
		var v uint32
		if v, err = d.readUint32(); err != nil {
			return nil, err
		}
		endOffset = uint64(v)
		if v, err = d.readUint32(); err != nil {
			return nil, err
		}
		numProps = uint64(v)
		if v, err = d.readUint32(); err != nil {
			return nil, err
		}
		propListLen = uint64(v)
	}

	nameLen, err := d.readUint8()
	if err != nil {
		return nil, err
	}
	if endOffset == 0 && numProps == 0 && propListLen == 0 && nameLen == 0 {
		return nil, nil
	}

	nameBytes, err := d.readBytes(int(nameLen))
	if err != nil {
		return nil, err
	}
	name := string(nameBytes)

	propStart := d.pos
	props := make([]any, 0, numProps)
	for i := uint64(0); i < numProps; i++ {
		v, err := d.readProperty()
		if err != nil {
			return nil, fmt.Errorf("fbxbinary: node %q property %d: %w", name, i, err)
		}
		props = append(props, v)
	}
	if got := uint64(d.pos - propStart); got != propListLen {
		return nil, fmt.Errorf("fbxbinary: node %q property list length mismatch: header says %d, read %d", name, propListLen, got)
	}

	var children []*Node
	for uint64(d.pos) < endOffset {
		c, err := d.readNode()
		if err != nil {
			return nil, err
		}
		if c == nil {
			break
		}
		children = append(children, c)
	}
	if uint64(d.pos) != endOffset {
		return nil, fmt.Errorf("fbxbinary: node %q end offset mismatch: header says %d, read to %d", name, endOffset, d.pos)
	}

	return &Node{Name: name, Props: props, Children: children}, nil
}

func (d *decoder) readProperty() (any, error) {
	code, err := d.readUint8()
	if err != nil {
		return nil, err
	}
	switch code {
	case 'Y':
		b, err := d.readBytes(2)
		if err != nil {
			return nil, err
		}
		return int16(binary.LittleEndian.Uint16(b)), nil
	case 'C':
		v, err := d.readUint8()
		if err != nil {
			return nil, err
		}
		return v != 0, nil
	case 'I':
		v, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return int32(v), nil
	case 'F':
		v, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(v), nil
	case 'D':
		v, err := d.readUint64()
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(v), nil
	case 'L':
		v, err := d.readUint64()
		if err != nil {
			return nil, err
		}
		return int64(v), nil
	case 'S':
		n, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		b, err := d.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		return decodeName(string(b)), nil
	case 'R':
		n, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		b, err := d.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	case 'f':
		raw, n, err := d.readArrayPayload(4)
		if err != nil {
			return nil, err
		}
		out := make([]float32, n)
		for i := range out {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		return out, nil
	case 'd':
		raw, n, err := d.readArrayPayload(8)
		if err != nil {
			return nil, err
		}
		out := make([]float64, n)
		for i := range out {
			out[i] = math.Float64frombits(binary.LittleEndian.Uint64(raw[i*8:]))
		}
		return out, nil
	case 'l':
		raw, n, err := d.readArrayPayload(8)
		if err != nil {
			return nil, err
		}
		out := make([]int64, n)
		for i := range out {
			out[i] = int64(binary.LittleEndian.Uint64(raw[i*8:]))
		}
		return out, nil
	case 'i':
		raw, n, err := d.readArrayPayload(4)
		if err != nil {
			return nil, err
		}
		out := make([]int32, n)
		for i := range out {
			out[i] = int32(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		return out, nil
	case 'b':
		raw, n, err := d.readArrayPayload(1)
		if err != nil {
			return nil, err
		}
		out := make([]bool, n)
		for i := range out {
			out[i] = raw[i] != 0
		}
		return out, nil
	default:
		return nil, fmt.Errorf("fbxbinary: unsupported property type code %q", code)
	}
}

// readArrayPayload reads an array property's length/encoding/compressed-length
// header and returns the decompressed element bytes (elemSize*length bytes)
// along with the element count.
func (d *decoder) readArrayPayload(elemSize int) (raw []byte, length uint32, err error) {
	length, err = d.readUint32()
	if err != nil {
		return nil, 0, err
	}
	encoding, err := d.readUint32()
	if err != nil {
		return nil, 0, err
	}
	compLen, err := d.readUint32()
	if err != nil {
		return nil, 0, err
	}
	data, err := d.readBytes(int(compLen))
	if err != nil {
		return nil, 0, err
	}

	switch encoding {
	case 0:
		if want := int(length) * elemSize; len(data) != want {
			return nil, 0, fmt.Errorf("fbxbinary: uncompressed array payload length mismatch: got %d bytes, want %d", len(data), want)
		}
		return data, length, nil
	case 1:
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, 0, fmt.Errorf("fbxbinary: zlib reader init: %w", err)
		}
		defer func() { _ = zr.Close() }()
		out, err := io.ReadAll(zr)
		if err != nil {
			return nil, 0, fmt.Errorf("fbxbinary: zlib decompress: %w", err)
		}
		if want := int(length) * elemSize; len(out) != want {
			return nil, 0, fmt.Errorf("fbxbinary: decompressed array payload length mismatch: got %d bytes, want %d", len(out), want)
		}
		return out, length, nil
	default:
		return nil, 0, fmt.Errorf("fbxbinary: unsupported array encoding %d", encoding)
	}
}

// decodeName reverses binary FBX's "Class::Name" string encoding, which
// splits the pair on "\x00\x01" with tokens in reverse order (e.g.
// "55galdrum\x00\x01Geometry" back to "Geometry::55galdrum"). Strings without
// that separator pass through unchanged.
func decodeName(s string) string {
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
