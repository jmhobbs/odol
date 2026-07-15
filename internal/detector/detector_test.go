package detector

import (
	"bytes"
	"errors"
	"testing"
)

func TestDetectDemoSignature(t *testing.T) {
	t.Parallel()

	got, err := Detect(bytes.NewReader([]byte("SP3D")))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	want := P3DFormat{
		Family:       P3DFamilyDemo,
		LODSignature: "SP3D",
	}
	if got != want {
		t.Fatalf("Detect() = %#v, want %#v", got, want)
	}
}

func TestDetectUnknownSignature(t *testing.T) {
	t.Parallel()

	_, err := Detect(bytes.NewReader([]byte("NOPEthis-is-junk")))
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("Detect() error = %v, want ErrUnknownFormat", err)
	}
}

func TestDetectShortHeader(t *testing.T) {
	t.Parallel()

	_, err := Detect(bytes.NewReader([]byte("ODOL")))
	if !errors.Is(err, ErrShortHeader) {
		t.Fatalf("Detect() error = %v, want ErrShortHeader", err)
	}
}

func TestP3DFormatStringUsesNormalizedODOLVersion(t *testing.T) {
	t.Parallel()

	got, err := Detect(bytes.NewReader([]byte{
		'O', 'D', 'O', 'L',
		'7', 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	if got.String() != "ODOLv7" {
		t.Fatalf("Detect().String() = %q, want %q", got.String(), "ODOLv7")
	}
}
