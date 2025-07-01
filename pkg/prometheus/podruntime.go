package prometheus

import (
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)


func CalculateStartAndEnd(result []interface{}, resolution time.Duration, window api.Window) (time.Time, time.Time) {

	valueArraySlice := result[0]
	valueArray, _ := valueArraySlice.(interface{})
	sTimestamp, _ := valueArray[0].(int64)

	valueArraySlice, _ = result.(interface{})
	valueArray, _ = valueArraySlice[len(result)-1].(interface{})
	eTimestamp, _ := valueArray[0].(int64)

	s := time.Unix(int64(sTimestamp), 0).UTC()
	e := time.Unix(int64(eTimestamp), 0).UTC()

	// The only corner-case here is what to do if you only get one timestamp.
	// This dilemma still requires the use of the resolution, and can be
	// clamped using the window. In this case, we want to honor the existence
	// of the pod by giving "one resolution" worth of duration, half on each
	// side of the given timestamp.
	if s.Equal(e) {
		s = s.Add(-1 * resolution / time.Duration(2))
		e = e.Add(resolution / time.Duration(2))
	}
	if s.Before(window.Start) {
		s = window.Start
	}
	if e.After(window.End) {
		e = window.End
	}
	// prevent end times in the future
	now := time.Now().UTC()
	if e.After(now) {
		e = now
	}

	return s, e
}