package constant

import "testing"

func TestPath2RelayModeRecognizesImageStudioGeneration(t *testing.T) {
	if got := Path2RelayMode("/pg/images/generations"); got != RelayModeImagesGenerations {
		t.Fatalf("expected image generation relay mode, got %d", got)
	}
}
