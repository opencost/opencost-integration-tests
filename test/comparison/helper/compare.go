package util

import (
	"math"
	"testing"
)

func CompareString(t *testing.T, fieldName, value1, value2 string) bool {
	if value1 != value2 {
		t.Errorf("field '%s' values did not match '%s' != '%s'", fieldName, value1, value2)
		return false
	}
	return true
}

func CompareLabels(t *testing.T, fieldName string, map1, map2 map[string]string) bool {
	if len(map1) != len(map2) {
		t.Errorf("field '%s' length did not match '%d' != '%d'", fieldName, len(map1), len(map2))
		return false
	}
	for key, value1 := range map1 {
		value2, ok := map2[key]
		if !ok {
			t.Errorf("field '%s' key '%s' not found in both maps", fieldName, key)
			return false
		}
		if value1 != value2 {
			t.Errorf("field '%s' key '%s' values did not match '%s' != '%s'", fieldName, key, value1, value2)
			return false
		}
	}
	return true
}

// compareValues compares two float64 values with a percentage tolerance
func CompareValues(t *testing.T, fieldName string, value1, value2 float64, tolerancePercent float64) bool {
	if value1 == 0 && value2 == 0 {
		return true
	}

	roundingPercent := (.00001 / value2) * 100

	// ignore diffs that are less than the rounding error
	absDiff := math.Abs(value1 - value2)
	if absDiff <= .00001 {
		return true
	}

	// Calculate the percentage difference
	diff := math.Abs((value1 / value2) - 1)
	if value2 == 0 {
		diff = 1
	}
	percentDiff := diff * 100
	if percentDiff < roundingPercent {
		percentDiff = 0
	}

	if percentDiff-roundingPercent > tolerancePercent {
		t.Errorf("%s values differ by %.2f%% (value1: %.5f, value2: %.5f)",
			fieldName, percentDiff, value1, value2)
		return false
	}
	return true
}
