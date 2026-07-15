package fbxexport

import (
	"bytes"
	"os"

	"github.com/jmhobbs/odol/internal/model"
)

// Marshal converts a model to binary FBX bytes, selecting the
// lowest-resolution LOD.
func Marshal(m *model.Model) ([]byte, error) {
	s, err := buildScene(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeBinary(&buf, s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteFile converts a model to binary FBX and writes it to path.
func WriteFile(path string, m *model.Model) error {
	data, err := Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// MarshalASCII converts a model to ASCII FBX bytes, selecting the
// lowest-resolution LOD.
func MarshalASCII(m *model.Model) ([]byte, error) {
	s, err := buildScene(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeASCII(&buf, s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteASCIIFile converts a model to ASCII FBX and writes it to path.
func WriteASCIIFile(path string, m *model.Model) error {
	data, err := MarshalASCII(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
