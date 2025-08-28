package api

import "time"

type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (window *Window) RunTime() float64 {
  
	runTime := window.End.Sub(window.Start).Minutes()

	return runTime
}

func ExpandTimeRange(current *Window, other *Window) *Window {

	if current == nil || other == nil {
		return current
	}

	if other.Start.Before(current.Start) {
		current.Start = other.Start
	}

	if other.End.After(current.End) {
		current.End = other.End
	}

	return current
}
