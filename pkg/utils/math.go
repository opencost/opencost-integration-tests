package utils

import (
	"math"
	"regexp"
	"strconv"
	"fmt"
)

func AreWithinPercentage(num1, num2, tolerance float64) (bool, float64) {
	// Checks if two numbers are within a certain absolute range of each other.
	if num1 == 0 && num2 == 0 {
		return true, 0
	}

	tolerance = math.Abs(tolerance)
	diff := math.Abs(num1 - num2)
	reference := math.Max(math.Abs(num1), math.Abs(num2))

	diff_percent := (diff / reference) * 100
	
	return (diff <= (reference * tolerance)), diff_percent
}

func ConvertToHours(minutes float64) float64 {
	// Convert Time from Minutes to Hours
	return minutes / 60
}

func RoundUpToTwoDecimals(num float64) float64 {

	temp := num * 100
	roundedTemp := math.Round(temp)
	return roundedTemp / 100
}


// extractNumericPrefix extracts the leading numeric part from a string.
// It handles both integers and floating-point numbers.
func ExtractNumericPrefix(s string) (float64, error) {
	// Regex to match a number at the beginning of the string.
	// `^` asserts position at the start of the string.
	// `\d+` matches one or more digits.
	// `(\.\d+)?` optionally matches a decimal point followed by one or more digits.
	re := regexp.MustCompile(`^(\d+(\.\d+)?)`)
	match := re.FindStringSubmatch(s)

	if len(match) > 1 {
		// The captured numeric string is in the first capturing group (index 1).
		numStr := match[1]
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse numeric part '%s' to float: %w", numStr, err)
		}
		return val, nil
	}
	return 0, fmt.Errorf("no numeric prefix found in string: '%s'", s)
}