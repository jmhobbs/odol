package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func TestConfirmContinueAcceptsY(t *testing.T) {
	var out bytes.Buffer
	err := confirmContinue(strings.NewReader("y\n"), &out)
	assert.NoError(t, err)
}

func TestConfirmContinueAcceptsYes(t *testing.T) {
	var out bytes.Buffer
	err := confirmContinue(strings.NewReader("Yes\n"), &out)
	assert.NoError(t, err)
}

func TestConfirmContinueRejectsOther(t *testing.T) {
	var out bytes.Buffer
	err := confirmContinue(strings.NewReader("n\n"), &out)
	assert.ErrorIs(t, err, errCancelled)
}

func TestConfirmContinueRejectsEmptyLine(t *testing.T) {
	var out bytes.Buffer
	err := confirmContinue(strings.NewReader("\n"), &out)
	assert.ErrorIs(t, err, errCancelled)
}

func TestConfirmContinueRejectsEOF(t *testing.T) {
	var out bytes.Buffer
	err := confirmContinue(strings.NewReader(""), &out)
	assert.ErrorIs(t, err, errCancelled)
}

func TestIsInteractiveFalseForRegularFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "notatty")
	require.NoError(t, os.WriteFile(tmp, []byte("x"), 0o644))
	f, err := os.Open(tmp)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	assert.False(t, isInteractive(f))
}

// TestIsInteractiveFalseForDevNull guards against the specific bug a naive
// os.ModeCharDevice check has: /dev/null is itself a character device, so
// that heuristic alone misreports the exact "--lod ... < /dev/null"
// scripted case this feature exists to handle as interactive.
func TestIsInteractiveFalseForDevNull(t *testing.T) {
	f, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	assert.False(t, isInteractive(f))
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read must not be called")
}

func TestNewConfirmFuncSkipsPromptWhenSkipPromptTrue(t *testing.T) {
	var out bytes.Buffer
	confirm := newConfirmFunc(&out, errorReader{}, true)

	err := confirm("lod.fbx", model.Vector3{X: 1, Y: 2, Z: 3})
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "lod.fbx")
}

func TestNewConfirmFuncPromptsWhenNotSkipped(t *testing.T) {
	var out bytes.Buffer
	confirm := newConfirmFunc(&out, strings.NewReader("y\n"), false)

	err := confirm("lod.fbx", model.Vector3{X: 1, Y: 2, Z: 3})
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "continue")
}

func TestNewConfirmFuncPromptDeclines(t *testing.T) {
	var out bytes.Buffer
	confirm := newConfirmFunc(&out, strings.NewReader("n\n"), false)

	err := confirm("lod.fbx", model.Vector3{X: 1, Y: 2, Z: 3})
	assert.ErrorIs(t, err, errCancelled)
}

// The reference model ships only with this repo's private sample assets, so
// the preview names it in words rather than by a path users can't open.
func TestPrintSizePreviewNamesReferenceModelWithoutFilePath(t *testing.T) {
	var out bytes.Buffer
	printSizePreview(&out, "lod.fbx", model.Vector3{X: 0.3, Y: 0.4, Z: 0.3})

	text := out.String()
	assert.Contains(t, text, "measured from the male player model")
	assert.NotContains(t, text, ".p3d")
}
