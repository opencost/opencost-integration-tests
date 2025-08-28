package prometheus

// import (
// 	"time"
// )

// // IntervalPoint describes a start or end of a window of time
// // Currently, this used in PVC-pod relations to detect/calculate
// // coefficients for PV cost when a PVC is shared between pods.
// type IntervalPoint struct {
// 	Time      time.Time
// 	PointType string
// 	Key       PodKey
// }

// // IntervalPoints describes a slice of IntervalPoint structs
// type IntervalPoints []IntervalPoint

// // Requisite functions for implementing sort.Sort for
// // IntervalPointList
// func (ips IntervalPoints) Len() int {
// 	return len(ips)
// }

// func (ips IntervalPoints) Less(i, j int) bool {
// 	if ips[i].Time.Equal(ips[j].Time) {
// 		return ips[i].PointType == "start" && ips[j].PointType == "end"
// 	}
// 	return ips[i].Time.Before(ips[j].Time)
// }

// func (ips IntervalPoints) Swap(i, j int) {
// 	ips[i], ips[j] = ips[j], ips[i]
// }