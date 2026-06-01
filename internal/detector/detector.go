package detector

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	ErrShortHeader   = errors.New("p3d header is too short")
	ErrUnknownFormat = errors.New("unknown p3d format")
)

type P3DFamily string

const (
	P3DFamilyDemo P3DFamily = "DEMO"
	P3DFamilyMLOD P3DFamily = "MLOD"
	P3DFamilyODOL P3DFamily = "ODOL"
)

type P3DFormat struct {
	Family       P3DFamily
	Version      uint32
	LODSignature string
}

func (f P3DFormat) String() string {
	switch f.Family {
	case P3DFamilyMLOD:
		if f.LODSignature == "" {
			return "MLOD"
		}
		return fmt.Sprintf("MLOD/%s", f.LODSignature)
	case P3DFamilyDemo:
		if f.LODSignature == "" {
			return "DEMO"
		}
		return fmt.Sprintf("DEMO/%s", f.LODSignature)
	case P3DFamilyODOL:
		return fmt.Sprintf("ODOLv%d", f.Version)
	default:
		return "UNKNOWN"
	}
}

func Detect(r io.Reader) (P3DFormat, error) {
	signatureBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, signatureBytes); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return P3DFormat{}, ErrShortHeader
		}
		return P3DFormat{}, err
	}

	signature := string(signatureBytes)

	switch signature {
	case "ODOL":
		header := make([]byte, 8)
		if _, err := io.ReadFull(r, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return P3DFormat{}, ErrShortHeader
			}
			return P3DFormat{}, err
		}

		rawVersion := binary.LittleEndian.Uint32(header[:4])

		return P3DFormat{
			Family:  P3DFamilyODOL,
			Version: normalizeODOLVersion(rawVersion),
		}, nil
	case "MLOD":
		header := make([]byte, 12)
		if _, err := io.ReadFull(r, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return P3DFormat{}, ErrShortHeader
			}
			return P3DFormat{}, err
		}

		return P3DFormat{
			Family:       P3DFamilyMLOD,
			Version:      binary.LittleEndian.Uint32(header[:4]),
			LODSignature: string(header[8:12]),
		}, nil
	case "SP3D", "SP3X", "P3DM":
		return P3DFormat{
			Family:       P3DFamilyDemo,
			LODSignature: signature,
		}, nil
	default:
		return P3DFormat{}, fmt.Errorf("%w: %q", ErrUnknownFormat, signature)
	}
}

func normalizeODOLVersion(version uint32) uint32 {
	if version >= '0' && version <= '9' {
		return version - '0'
	}

	return version
}

func DetectFile(path string) (P3DFormat, error) {
	file, err := os.Open(path)
	if err != nil {
		return P3DFormat{}, err
	}
	defer func() {
		_ = file.Close()
	}()

	return Detect(file)
}
