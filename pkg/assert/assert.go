package assert

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

// ErrorPct returns the difference between exp and act as a percent of exp.
// E.g. exp=5.0, act=4.0 => 0.2, i.e., 20%
func ErrorPct(exp, act float64) float64 {
	if exp == 0.0 && act != 0.0 {
		return 1.0
	}

	return math.Abs(exp-act) / exp
}

// IsApproximatelyWithThreshold asserts approximate equality within a given
// threshold; e.g. threashold = 0.01 asserts y is close to x, within 1% of x.
func IsApproximatelyWithThreshold(exp, act, threshold float64) bool {
	delta := exp * threshold
	if delta < 0.00001 {
		delta = 0.00001
	}
	return math.Abs(exp-act) < delta
}

// IsApproximately uses the env var for proximity "threshold" to determine
// approximate equality.
func IsApproximately(exp, act float64) bool {
	return IsApproximatelyWithThreshold(exp, act, env.GetApproxThreshold())
}

// Asserter provides assertion testing helpers.
type Asserter struct {
	ApproxThreshold float64
	LogPrefix       string
	T               *testing.T
}

// NewAsserter instantiates a new Asserter (helper functions) that takes a test
// state manager (*testing.T) for reporting failures.
func NewAsserter(t *testing.T) *Asserter {
	return &Asserter{
		ApproxThreshold: env.GetApproxThreshold(),
		LogPrefix:       "",
		T:               t,
	}
}

// AssertApproximately asserts that exp ~= act within the asserter's configured
// threshold and errors if that condition fails.
func (a *Asserter) AssertApproximately(exp, act float64, msg string) {
	a.T.Helper()
	if !a.IsApproximately(exp, act) {
		a.Errorf("%s: exp %f !~= %f act (%.2f%% error)", msg, exp, act, ErrorPct(exp, act)*100)
	}
}

// AssertEqualFloat64 asserts that exp == act and errors if that condition fails.
func (a *Asserter) AssertEqualFloat64(exp, act float64, msg string) {
	a.T.Helper()
	if exp != act {
		a.Errorf("%s: exp %f !== %f act (%.2f%% error)", msg, exp, act, ErrorPct(exp, act)*100)
	}
}

// AssertNonZeroFloat64 asserts that exp is non zero and errors if it is equal to float zero value.
func (a *Asserter) AssertNonZeroFloat64(exp float64, msg string) {
	a.T.Helper()
	if exp != 0.0 {
		return
	}
	a.Errorf("%s: AssertNonZeroFloat64 failed to validate non zero value", msg)
}

// AssertEqualInt asserts that exp == act and errors if that condition fails.
func (a *Asserter) AssertEqualInt(exp, act int, msg string) {
	a.T.Helper()
	if exp != act {
		a.Errorf("%s: exp %d !== %d act (%+d error)", msg, exp, act, act-exp)
	}
}

// AssertEqualString asserts that exp == act and errors if that condition fails.
func (a *Asserter) AssertEqualString(exp, act string, msg string) {
	a.T.Helper()
	if exp != act {
		a.Errorf("%s: exp \"%s\" !== \"%s\" act", msg, exp, act)
	}
}

// AssertEqualTime asserts that exp == act and errors if that condition fails.
func (a *Asserter) AssertEqualTime(exp, act time.Time, msg string) {
	a.T.Helper()
	if exp != act {
		a.Errorf("%s: exp \"%s\" !== \"%s\" act", msg, exp, act)
	}
}

// AssertEqualSlice asserts that exp == act and errors if that condition fails.
func AssertEqualSlice[S ~[]E, E comparable](a *Asserter, exp, act S, msg string) {
	a.T.Helper()
	if !slices.Equal(exp, act) {
		a.Errorf("%s: exp \"%s\" !== \"%s\" act", msg, exp, act)
	}
}

// AssertEqualMap asserts that exp == act and errors if that condition fails.
func AssertEqualMap[M1, M2 ~map[K]V, K, V comparable](a *Asserter, exp M1, act M2, msg string) {
	a.T.Helper()
	if !maps.Equal(exp, act) {
		a.Errorf("%s: exp \"%s\" !== \"%s\" act", msg, exp, act)
	}
}

// Errorf calls Errorf on asserter's T, appending the log prefix if it exists.
func (a *Asserter) Errorf(format string, args ...any) {
	a.T.Helper()
	if a.LogPrefix == "" {
		a.T.Errorf(format, args...)
	} else {
		msg := fmt.Sprintf(format, args...)
		a.T.Errorf("%s: %s", a.LogPrefix, msg)
	}
}

// IsApproximately uses the env var for proximity "threshold" to determine
// approximate equality.
func (a *Asserter) IsApproximately(exp, act float64) bool {
	return IsApproximatelyWithThreshold(exp, act, a.ApproxThreshold)
}
