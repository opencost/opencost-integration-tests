package env

import (
	"os"
	"testing"
)

func TestGetDataResolutionMinutes(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		os.Unsetenv("OPENCOST_DATA_RESOLUTION_MINUTES")
		if got := GetDataResolutionMinutes(); got != 1 {
			t.Fatalf("expected default 1, got %d", got)
		}
	})

	t.Run("override", func(t *testing.T) {
		t.Setenv("OPENCOST_DATA_RESOLUTION_MINUTES", "5")
		if got := GetDataResolutionMinutes(); got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})
}
