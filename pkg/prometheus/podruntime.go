package prometheus

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"time"
)

type PodKey struct {
	Namespace		string
	Pod				string
	UID				string
}

type PersistentVolume struct {
	Name 		    string
	Start 		    time.Time
	End 		    time.Time
	CostPerGiBHour  float64
	ProviderID	    string
	PVBytes		    float64
	StorageClass	string
}

type PersistentVolumeClaim struct {
	PersistentVolume			    *PersistentVolume
	Namespace						string
	PersistentVolumeClaimName		string
	Start							time.Time
	End								time.Time
	RequestedBytes					float64
	Mounted							bool
}

type PVAllocations struct {
	ByteHours  float64
	Cost       float64
	ProviderID string
}

type PodData struct {
	Pod        string
	Namespace  string
	Start      time.Time
	End        time.Time
	Minutes    float64
	Containers map[string]map[string]*PVAllocations // One to Many Relationship for Each Container. Conatainer --> Multiple Persistent Volumes

}

func CalculateStartAndEnd(result []DataPoint, resolution time.Duration, window api.Window) (time.Time, time.Time) {
	// Start and end for a range vector are pulled from the timestamps of the
	// first and final values in the range. There is no "offsetting" required
	// of the start or the end, as we used to do. If you query for a duration
	// of time that is divisible by the given resolution, and set the end time
	// to be precisely the end of the window, Prometheus should give all the
	// relevant timestamps.
	//
	// E.g. avg(kube_pod_container_status_running{}) by (pod, namespace)[1h:1m]
	// with time=01:00:00 will return, for a pod running the entire time,
	// 61 timestamps where the first is 00:00:00 and the last is 01:00:00.
	s := time.Unix(int64(result[0].Timestamp), 0).UTC()
	e := time.Unix(int64(result[len(result)-1].Timestamp), 0).UTC()

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
