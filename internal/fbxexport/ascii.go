package fbxexport

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type fbxWriter struct {
	w     *bufio.Writer
	depth int
	err   error
}

func newFBXWriter(w io.Writer) *fbxWriter {
	return &fbxWriter{w: bufio.NewWriter(w)}
}

func (fw *fbxWriter) flush() error {
	if fw.err != nil {
		return fw.err
	}
	return fw.w.Flush()
}

// writef writes an indented line. format must be a hardcoded literal.
func (fw *fbxWriter) writef(format string, args ...any) {
	if fw.err != nil {
		return
	}
	indent := strings.Repeat("\t", fw.depth)
	line := fmt.Sprintf(format, args...)
	_, fw.err = fmt.Fprint(fw.w, indent+line+"\n")
}

// rawLine writes an indented line without format interpretation.
// Use for data strings that may contain % characters.
func (fw *fbxWriter) rawLine(s string) {
	if fw.err != nil {
		return
	}
	indent := strings.Repeat("\t", fw.depth)
	_, fw.err = fmt.Fprint(fw.w, indent+s+"\n")
}

func (fw *fbxWriter) blankLine() {
	if fw.err != nil {
		return
	}
	_, fw.err = fmt.Fprintln(fw.w)
}

func (fw *fbxWriter) openBlock(header string) {
	fw.rawLine(header + " {")
	fw.depth++
}

func (fw *fbxWriter) closeBlock() {
	fw.depth--
	fw.writef("}")
}

func (fw *fbxWriter) floatArray(name string, values []float64) {
	fw.writef("%s: *%d {", name, len(values))
	fw.depth++
	if len(values) > 0 {
		fw.rawLine("a: " + joinFloat64s(values))
	}
	fw.depth--
	fw.writef("}")
}

func (fw *fbxWriter) int32Array(name string, values []int32) {
	fw.writef("%s: *%d {", name, len(values))
	fw.depth++
	if len(values) > 0 {
		fw.rawLine("a: " + joinInt32s(values))
	}
	fw.depth--
	fw.writef("}")
}

// writeASCII builds the generic FBX node tree for s and renders it as
// ASCII FBX 7.4 text.
func writeASCII(w io.Writer, s *meshScene) error {
	fw := newFBXWriter(w)

	fw.rawLine("; FBX 7.4.0 project file")
	fw.rawLine("; Creator: odol-convert")
	fw.blankLine()

	nodes := buildDocumentNodes(s, time.Now())
	for i, n := range nodes {
		renderNode(fw, n)
		if i < len(nodes)-1 {
			fw.blankLine()
		}
	}

	return fw.flush()
}

func joinFloat64s(values []float64) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(v, 'f', 6, 64))
	}
	return sb.String()
}

func joinInt32s(values []int32) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(int64(v), 10))
	}
	return sb.String()
}

func escapeFBXString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
