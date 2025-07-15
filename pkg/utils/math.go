package utils

import (
	"math"
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
