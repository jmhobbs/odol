package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// failingReader fails the test if the CLI reads stdin when it should not.
type failingReader struct {
	t *testing.T
}

func (r failingReader) Read([]byte) (int, error) {
	r.t.Fatalf("stdin was read but pauseOnExit is false")
	return 0, nil
}

func TestRunCLIDoesNotPauseByDefault(t *testing.T) {
	var stderr bytes.Buffer

	exitCode := runCLI(nil, cliOptions{stdin: failingReader{t: t}, stderr: &stderr})

	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr.String(), "usage:")
	assert.NotContains(t, stderr.String(), pausePrompt)
}
