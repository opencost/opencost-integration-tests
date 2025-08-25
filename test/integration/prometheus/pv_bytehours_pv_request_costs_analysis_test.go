package prometheus

// Description: Checks 
// - PVCost
// - PVBytesHours

// Methodology
// - Build Pod Map
// - Build Persistent Volume Map
// - Build Persistent Volume Claim Map
// - Calculate Costs Pod/Persistent Volume

// Note:
// Container Costs are not real costs but generated costs. They are obtained by equally distributing pod costs to all pod containers. This 
// is the not the best approach as init containers can also contribute to the bytehours and cost.
//
// IngestUID is a flag you can set to calculate by Pod and UID. This way Pod costs are calculated separately based on UID and then aggregated by Pod for the final results.
// This feature is currently not supported by OpenCost

import (
	"fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"regexp"
	"sort"
	"strings"
	"strconv"
	"testing"
	"time"
)


// Default Opencost Resolution
const Resolution = "1m"

// Accepted Difference
const Tolerance = 0.05

const KiB = 1024.0
const MiB = 1024.0 * KiB
const GiB = 1024.0 * MiB
const TiB = 1024.0 * GiB
const PiB = 1024.0 * TiB
const PV_USAGE_SANITY_LIMIT_BYTES = 10.0 * PiB

const NegligibleCost = 0.01

type PodKey struct {
	Namespace		string
	Pod				string
	UID				string
}

type PersistentVolumeClaimKey struct {
	Namespace                 string
	PersistentVolumeClaimName string
}

type PersistentVolume struct {
	Name 		    string
	Window			api.Window
	CostPerGiBHour  float64
	ProviderID	    string
	PVBytes		    float64
	StorageClass	string
}

type PersistentVolumeClaim struct {
	PersistentVolume			    *PersistentVolume
	Namespace						string
	PersistentVolumeClaimName		string
	Window							api.Window
	RequestedBytes					float64
	Mounted							bool
}

type PersistentVolumeAllocations struct {
	ByteHours  float64
	Cost       float64
	ProviderID string
}

type PodData struct {
	Pod        		string
	Namespace  		string
	Window	       	api.Window
	NumContainers  	int
	Allocations 	map[string]*PersistentVolumeAllocations
}


// This function is used to match the query pattern in allocation
// If using a version of Prometheus where the resolution needs duration offset,
//
// E.g. avg(node_total_hourly_cost{}) by (node, provider_id)[60m:5m] with
// time=01:00:00 will return, for a node running the entire time, 12
// timestamps where the first is 00:05:00 and the last is 01:00:00.
// However, OpenCost expects for there to be 13 timestamps where the first
// begins at 00:00:00. To achieve this, we must modify our query to
// avg(node_total_hourly_cost{}) by (node, provider_id)[65m:5m]

// Allocation Working Style
// 24h: [2025-08-21T21:00:00+0000, 2025-08-22T21:00:00+0000)
// 1440m: [2025-08-21T20:51:00+0000, 2025-08-22T20:51:00+0000)

// 24h in allocation gets records from the first minute of the hour but promethues 3.0 and above considers the first point after the start hour.
// To not undercount values, we increase the range to counter promethues's behavior

func getOffsetAdjustedQueryWindow(window string) (string) {

	// This function is specifically designed for window is [0-9]h format and resolution in [0-9]m. 
	// Please upgrade this function if you want to support more time ranges or special keywords.
	window_int, _ := utils.ExtractNumericPrefix(window)
	resolution_int, _ := utils.ExtractNumericPrefix(Resolution)

	window_offset := strconv.Itoa(int(window_int) * 60 + int(resolution_int))
	window_offset_string := fmt.Sprintf("%sm", window_offset)

	return window_offset_string
}

func queryPods(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// --------------------------------------
	// Build Pod Map
	// --------------------------------------

	// Query all running pod information
	// avg(kube_pod_container_status_running{} != 0)
	// by
	// (pod, namespace)[24h:5m]

	client := prometheus.NewClient()
	promPodInfoInput := prometheus.PrometheusInput{}

	promPodInfoInput.Metric = "kube_pod_container_status_running"
	promPodInfoInput.Filters = map[string]string{}
	promPodInfoInput.MetricNotEqualTo = "0"
	promPodInfoInput.AggregateBy = []string{"pod", "namespace"}
	promPodInfoInput.Function = []string{"avg"}
	promPodInfoInput.AggregateWindow = getOffsetAdjustedQueryWindow(window)
	promPodInfoInput.AggregateResolution = Resolution
	promPodInfoInput.Time = &endTime

	podInfo, err := client.RunPromQLQuery(promPodInfoInput)
	return podInfo, err
}

func queryPodsUID(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// --------------------------------------
	// Build UID Pod Map 
	// --------------------------------------

	// Query all running pod information
	// avg(kube_pod_container_status_running{} != 0)
	// by
	// (pod, namespace, uid)[24h:5m]

	client := prometheus.NewClient()
	promPodInfoInput := prometheus.PrometheusInput{}

	promPodInfoInput.Metric = "kube_pod_container_status_running"
	promPodInfoInput.Filters = map[string]string{}
	promPodInfoInput.MetricNotEqualTo = "0"
	promPodInfoInput.AggregateBy = []string{"pod", "namespace", "uid"}
	promPodInfoInput.Function = []string{"avg"}
	promPodInfoInput.AggregateWindow = getOffsetAdjustedQueryWindow(window)
	promPodInfoInput.AggregateResolution = Resolution
	promPodInfoInput.Time = &endTime

	podInfo, err := client.RunPromQLQuery(promPodInfoInput)

	return podInfo, err
}

func queryPVActiveMins(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PVActiveMins
	// Description: Get Alive Time for the Persistent Volume

	// avg(kube_persistentvolume_capacity_bytes{%s}) by (%s, persistentvolume)[%s:%dm]`
	// ----------------------------------------------
	
	client := prometheus.NewClient()
	promPVRunTime := prometheus.PrometheusInput{}

	promPVRunTime.Metric = "kube_persistentvolume_capacity_bytes"
	promPVRunTime.AggregateBy = []string{"persistentvolume"}
	promPVRunTime.Function = []string{"avg"}
	promPVRunTime.AggregateWindow = getOffsetAdjustedQueryWindow(window)
	promPVRunTime.AggregateResolution = Resolution
	promPVRunTime.Time = &endTime

	pvRunTime, err := client.RunPromQLQuery(promPVRunTime)

	return pvRunTime, err
}

func queryPVCapacityBytes(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PVBytes
	// Description: Get PersistentVolume Capacity

	// avg(avg_over_time(kube_persistentvolume_capacity_bytes{%s}[%s]))
	// by
	// (persistentvolume, %s)`
	// ----------------------------------------------

	client := prometheus.NewClient()
	promPVBytes := prometheus.PrometheusInput{}

	promPVBytes.Metric = "kube_persistentvolume_capacity_bytes"
	promPVBytes.AggregateBy = []string{"persistentvolume"}
	promPVBytes.Function = []string{"avg_over_time", "avg"}
	promPVBytes.QueryWindow = getOffsetAdjustedQueryWindow(window)
	promPVBytes.Time = &endTime

	pvBytes, err := client.RunPromQLQuery(promPVBytes)

	return pvBytes, err
}

func queryPVCostPerGibHour(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PVCostPerGibHour
	// Description: Get Cost for Every Byte Used in an Hour in GigaBytes

	// avg(avg_over_time(pv_hourly_cost{%s}[%s]))
	// by
	// (%s, persistentvolume, volumename, provider_id)
	// ----------------------------------------------

	client := prometheus.NewClient()
	promCostPerGiBHour := prometheus.PrometheusInput{}

	promCostPerGiBHour.Metric = "pv_hourly_cost"
	promCostPerGiBHour.AggregateBy = []string{"persistentvolume", "volumename", "provider_id"}
	promCostPerGiBHour.Function = []string{"avg_over_time", "avg"}
	promCostPerGiBHour.QueryWindow = getOffsetAdjustedQueryWindow(window)
	promCostPerGiBHour.Time = &endTime

	pvCostPerGiBHour, err := client.RunPromQLQuery(promCostPerGiBHour)

	return pvCostPerGiBHour, err
}

func queryPVMeta(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PVMeta
	// Description: Persistent Volume Information

	// avg(avg_over_time(kubecost_pv_info{%s}[%s]))
	// by
	// (%s, storageclass, persistentvolume, provider_id)
	// ----------------------------------------------
	
	client := prometheus.NewClient()
	promPVMeta := prometheus.PrometheusInput{}

	promPVMeta.Metric = "kubecost_pv_info"
	promPVMeta.AggregateBy = []string{"storageclass", "persistentvolume", "provider_id"}
	promPVMeta.Function = []string{"avg_over_time", "avg"}
	promPVMeta.QueryWindow = getOffsetAdjustedQueryWindow(window)
	promPVMeta.Time = &endTime

	pvMeta, err := client.RunPromQLQuery(promPVMeta)

	return pvMeta, err
}

func queryPVCInfo(window string, endTime int64) (prometheus.PrometheusResponse, error) {

	// ----------------------------------------------
	// Metric: PVCInfo
	// Description: Persistent Volume Claim Information

	// avg(kube_persistentvolumeclaim_info{volumename != "", %s})
	// by
	// (persistentvolumeclaim, storageclass, volumename, namespace, %s)[%s:%dm]
	// ----------------------------------------------
	
	client := prometheus.NewClient()
	promPVCInfo := prometheus.PrometheusInput{}

	promPVCInfo.Metric = "kube_persistentvolumeclaim_info"
	promPVCInfo.IgnoreFilters = map[string][]string{
		"volumename": {""},
	}
	promPVCInfo.AggregateBy = []string{"persistentvolumeclaim", "storageclass", "volumename", "namespace"}
	promPVCInfo.Function = []string{"avg"}
	promPVCInfo.AggregateWindow = getOffsetAdjustedQueryWindow(window)
	promPVCInfo.AggregateResolution = Resolution
	promPVCInfo.Time = &endTime

	pvcInfo, err := client.RunPromQLQuery(promPVCInfo)
	
	return pvcInfo, err
}

func queryPVCRequestedBytes(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PVCRequestedBytes
	// Description: Persistent Volume Claim Requested Bytes

	// avg(avg_over_time(kube_persistentvolumeclaim_resource_requests_storage_bytes{%s}[%s]))
	// by
	// (persistentvolumeclaim, namespace, %s)
	// ----------------------------------------------
	
	client := prometheus.NewClient()
	promPVCRequestedBytes := prometheus.PrometheusInput{}

	promPVCRequestedBytes.Metric = "kube_persistentvolumeclaim_resource_requests_storage_bytes"
	promPVCRequestedBytes.AggregateBy = []string{"persistentvolumeclaim", "namespace"}
	promPVCRequestedBytes.Function = []string{"avg_over_time", "avg"}
	promPVCRequestedBytes.QueryWindow = getOffsetAdjustedQueryWindow(window)
	promPVCRequestedBytes.Time = &endTime

	pvcRequestedBytes, err := client.RunPromQLQuery(promPVCRequestedBytes)

	return pvcRequestedBytes, err
}

func queryPodPVCAllocation(window string, endTime int64) (prometheus.PrometheusResponse, error) {
	// ----------------------------------------------
	// Metric: PodPVCAllocation
	// Description: Pod Persistent Volume Claim Allocation

	// avg(avg_over_time(pod_pvc_allocation{%s}[%s]))
	// by
	// (persistentvolume, persistentvolumeclaim, pod, namespace, %s)
	// ----------------------------------------------

	client := prometheus.NewClient()
	promPodPVCAllocation := prometheus.PrometheusInput{}

	promPodPVCAllocation.Metric = "pod_pvc_allocation"
	promPodPVCAllocation.AggregateBy = []string{"persistentvolume", "persistentvolumeclaim", "pod", "namespace"}
	promPodPVCAllocation.Function = []string{"avg_over_time", "avg"}
	promPodPVCAllocation.QueryWindow = getOffsetAdjustedQueryWindow(window)
	promPodPVCAllocation.Time = &endTime

	podPVCAllocation, err := client.RunPromQLQuery(promPodPVCAllocation)

	return podPVCAllocation, err
}

func buildPodMap(IngestUID bool, window string, endTime int64, resolution time.Duration, queryWindow api.Window, t *testing.T) (map[PodKey]*PodData, map[PodKey][]PodKey) {
	
	var podInfo prometheus.PrometheusResponse
	var err error

	if !IngestUID {
		podInfo, err = queryPods(window, endTime)
	} else {
		podInfo, err = queryPodsUID(window, endTime)
	}
	
	if err != nil {
		t.Fatalf("Prometheus Query kube_pod_container_status_running failed: %v", err)
	}

	podMap := make(map[PodKey]*PodData)
	podUIDKeyMap := make(map[PodKey][]PodKey)

	for _, podInfoResponseItem := range podInfo.Data.Result {
		namespace := podInfoResponseItem.Metric.Namespace
		pod := podInfoResponseItem.Metric.Pod
		// container := podInfoResponseItem.Metric.Container
		uid := podInfoResponseItem.Metric.UID

		podKey := PodKey{
			Namespace: namespace,
			Pod:       pod,
		}

		// This is to account for pod replicas
		if IngestUID {
			if uid == "" {
				continue
			} else {
				newPodKey := PodKey{
					Namespace: namespace,
					Pod:       pod,
					UID:       uid,
				}

				podUIDKeyMap[podKey] = append(podUIDKeyMap[podKey], newPodKey)
				podKey = newPodKey
			}
		}

		s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, queryWindow)
		podWindow := api.Window{
			Start: s,
			End: e,
		}
		if pod == "test-install-speedtest-tracker-56944bbf5b-h76q7" {
			t.Logf("Start and End %v, %v, %v", s, e, podWindow.RunTime())
		}
		if thisPod, ok := podMap[podKey]; ok {
			thisPod.Window = *api.ExpandTimeRange(&thisPod.Window, &podWindow)
		} else {
			podMap[podKey] = &PodData{
				Pod: 		pod,
				Namespace:  namespace,
				Window: podWindow,
				Allocations: make(map[string]*PersistentVolumeAllocations),
			}
		}
	}

	// Sometimes podUIDKey map is not required in our testing
	return podMap, podUIDKeyMap
}

func buildPVMap(window string, endTime int64, resolution time.Duration, queryWindow api.Window, t *testing.T) (map[string]*PersistentVolume) {

	pvRunTime, err := queryPVActiveMins(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL kube_persistentvolume_capacity_bytes: %v", err)
	}

	pvCostPerGiBHour, err := queryPVCostPerGibHour(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL pv_hourly_cost: %v", err)
	}

	pvBytes, err := queryPVCapacityBytes(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL kube_persistentvolume_capacity_bytes: %v", err)
	}

	pvMeta, err := queryPVMeta(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL kubecost_pv_info: %v", err)
	}

	persistentVolumeMap := make(map[string]*PersistentVolume)

	// Start and End Times for a PV
	for _, promPVRunTimeItem := range pvRunTime.Data.Result {
		persistentVolumeName := promPVRunTimeItem.Metric.PersistentVolume
		s, e := prometheus.CalculateStartAndEnd(promPVRunTimeItem.Values, resolution, queryWindow)
		persistentVolumeMap[persistentVolumeName] = &PersistentVolume{
			Name:           persistentVolumeName,
			Window: 		api.Window{
				Start: s,
				End: e,
			},
			CostPerGiBHour: 0.0,
			ProviderID:     "",
		}
	}

	// CostPerGiBHour for a PV
	for _, promCostPerGiBHourItem := range pvCostPerGiBHour.Data.Result {
		persistentVolumeName := promCostPerGiBHourItem.Metric.PersistentVolume
		PVItem, ok := persistentVolumeMap[persistentVolumeName]
		if !ok {
			t.Errorf("PersistentVolume %s missing from kube_persistentvolume_capacity_bytes", persistentVolumeName)
			continue
		}
		PVItem.CostPerGiBHour = promCostPerGiBHourItem.Value.Value
	}

	// only add metadata for disks that exist in the other metrics
	for _, promPVMetaItem := range pvMeta.Data.Result {
		persistentVolumeName := promPVMetaItem.Metric.PersistentVolume
		providerId := promPVMetaItem.Metric.ProviderID

		if PVItem, ok := persistentVolumeMap[persistentVolumeName]; ok {
			if providerId != "" {
				PVItem.ProviderID = providerId
			}
		}
	}

	// Add PVBytes for PV
	for _, promPVBytesItem := range pvBytes.Data.Result {
		persistentVolumeName := promPVBytesItem.Metric.PersistentVolume
		PVItem, ok := persistentVolumeMap[persistentVolumeName]
		if !ok {
			// t.Errorf("PersistentVolume %s missing from kube_persistentvolume_capacity_bytes", persistentVolumeName)
			continue
		}
		PVItem.PVBytes = promPVBytesItem.Value.Value

		// PV usage exceeds sanity limit
		if PVItem.PVBytes > PV_USAGE_SANITY_LIMIT_BYTES {
			t.Logf("PV usage exceeds sanity limit, clamping to zero for %s", persistentVolumeName)
			PVItem.PVBytes = 0.0
		}
	}

	return persistentVolumeMap
}

func buildPVCMap(window string, endTime int64, resolution time.Duration, queryWindow api.Window, persistentVolumeMap map[string]*PersistentVolume, t *testing.T) (map[PersistentVolumeClaimKey]*PersistentVolumeClaim) {
	
	pvcInfo, err := queryPVCInfo(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL kube_persistentvolumeclaim_info: %v", err)
	}

	pvcRequestedBytes, err := queryPVCRequestedBytes(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL kube_persistentvolumeclaim_resource_requests_storage_byte: %v", err)
	}

	persistentVolumeClaimMap := make(map[PersistentVolumeClaimKey]*PersistentVolumeClaim)

	// Get PVC Information
	for _, promPVCInfoItem := range pvcInfo.Data.Result {
		persistentVolumeName := promPVCInfoItem.Metric.VolumeName
		persistentVolumeClaimName := promPVCInfoItem.Metric.PersistentVolumeClaim
		storageClass := promPVCInfoItem.Metric.StorageClass
		namespace := promPVCInfoItem.Metric.Namespace

		if namespace == "" || persistentVolumeClaimName == "" || persistentVolumeName == "" || storageClass == "" {
			t.Logf("PV Test: pvc info query result missing field")
			continue
		}

		pvItem, ok := persistentVolumeMap[persistentVolumeName]
		if !ok {
			continue
		}

		pvItem.StorageClass = storageClass

		// Get Start and End time for Persistent Volume Claim
		s, e := prometheus.CalculateStartAndEnd(promPVCInfoItem.Values, resolution, queryWindow)

		// Create a PersistentVolume Index and link the PersistentVolume Information
		persistentVolumeClaimKey := PersistentVolumeClaimKey{
			Namespace:                 namespace,
			PersistentVolumeClaimName: persistentVolumeClaimName,
		}

		persistentVolumeClaimMap[persistentVolumeClaimKey] = &PersistentVolumeClaim{
			Namespace:                 namespace,
			PersistentVolumeClaimName: persistentVolumeClaimName,
			PersistentVolume:          pvItem,
			Window:					   api.Window{
				Start: s,
				End: e,
			},
		}
	}

	// Add PVCRequestedBytes
	for _, promPVCRequestedBytesItem := range pvcRequestedBytes.Data.Result {
		persistentVolumeClaimName := promPVCRequestedBytesItem.Metric.PersistentVolumeClaim
		namespace := promPVCRequestedBytesItem.Metric.Namespace

		persistentVolumeClaimKey := PersistentVolumeClaimKey{
			Namespace:                 namespace,
			PersistentVolumeClaimName: persistentVolumeClaimName,
		}

		persistentVolumeClaimItem, ok := persistentVolumeClaimMap[persistentVolumeClaimKey]
		if !ok {
			continue
		}

		// Add Requested Bytes Information
		persistentVolumeClaimItem.RequestedBytes = promPVCRequestedBytesItem.Value.Value
	}

	return persistentVolumeClaimMap
}

func buildPodPVCMap(window string, endTime int64, podMap map[PodKey]*PodData, persistentVolumeMap map[string]*PersistentVolume, persistentVolumeClaimMap map[PersistentVolumeClaimKey]*PersistentVolumeClaim, IngestUID bool, podUIDKeyMap map[PodKey][]PodKey, t *testing.T) (map[PodKey][]*PersistentVolumeClaim) {
	
	podPVCAllocation, err := queryPodPVCAllocation(window, endTime)
	if err != nil {
		t.Fatalf("Error Occured while querying PromQL pod_pvc_allocation: %v", err)
	}

	podPVCMap := make(map[PodKey][]*PersistentVolumeClaim)

	for _, podPVCAllocationItem := range podPVCAllocation.Data.Result {

		namespace := podPVCAllocationItem.Metric.Namespace
		pod := podPVCAllocationItem.Metric.Pod
		persistentVolumeName := podPVCAllocationItem.Metric.PersistentVolume
		persistentVolumeClaimName := podPVCAllocationItem.Metric.PersistentVolumeClaim

		if namespace == "" || pod == "" || persistentVolumeName == "" || persistentVolumeClaimName == "" {
			t.Logf("PV Test: pvc allocation query result missing field")
			continue
		}

		podKey := PodKey{
			Namespace: namespace,
			Pod:       pod,
		}

		persistentVolumeClaimKey := PersistentVolumeClaimKey{
			Namespace:                 namespace,
			PersistentVolumeClaimName: persistentVolumeClaimName,
		}

		if _, ok := persistentVolumeMap[persistentVolumeName]; !ok {
			t.Logf("PV Test: pv missing for pvc allocation query result: %s", persistentVolumeName)
		}

		pvc, ok := persistentVolumeClaimMap[persistentVolumeClaimKey]
		if !ok {
			t.Logf("PV Test: pvc missing for from PVC alloctions prom query: %s", persistentVolumeClaimKey)
			continue
		}

		pvc.Mounted = true

		if IngestUID {
			for _, key := range podUIDKeyMap[podKey] {
				podPVCMap[key] = append(podPVCMap[key], pvc)
			}
		} else {
			podPVCMap[podKey] = append(podPVCMap[podKey], pvc)
		}
	}

	return podPVCMap
}

func applyPodPVCCosts(queryWindow api.Window, podMap map[PodKey]*PodData, podPVCMap map[PodKey][]*PersistentVolumeClaim, persistentVolumeClaimMap map[PersistentVolumeClaimKey]*PersistentVolumeClaim, t *testing.T) () {

	// For each persistent volume
	// Attach the pod along with a modified run time based on the persistent volume.
	// This interval is the intersection of the persistentvolume alive time and pod alive time intervals

	pvcPodWindowMap := make(map[PersistentVolumeClaimKey]map[PodKey]api.Window)

	for thisPodKey, thisPod := range podMap {
		// Get all persistent volume claims made by a namespace that a pod belongs to
		if pvcs, ok := podPVCMap[thisPodKey]; ok {
			for _, thisPVC := range pvcs {

				// Try to limit the usage interval of persistentclaim for this pod to the pod's window size
				s, e := thisPod.Window.Start, thisPod.Window.End

				if thisPVC.Window.Start.After(thisPod.Window.Start) {
					s = thisPVC.Window.Start
				}
				if thisPVC.Window.End.Before(thisPod.Window.End) {
					e = thisPVC.Window.End
				}

				thisPVCKey := PersistentVolumeClaimKey{
					Namespace:                 thisPVC.Namespace,
					PersistentVolumeClaimName: thisPVC.PersistentVolumeClaimName,
				}

				if pvcPodWindowMap[thisPVCKey] == nil {
					pvcPodWindowMap[thisPVCKey] = make(map[PodKey]api.Window)
				}
				pvcPodWindowMap[thisPVCKey][thisPodKey] = api.Window{
					Start: s,
					End:   e,
				}
			}
		}
	}

	for thisPVCKey, pvcPodWindowMap := range pvcPodWindowMap {

		// Sort the pod intervals in ascending order
		intervals := GetIntervalPointsFromWindows(pvcPodWindowMap)

		// Check for errors later
		pvc, _ := persistentVolumeClaimMap[thisPVCKey]

		sharedPVCCostCoefficients, err := GetPVCCostCoefficients(intervals, pvc, t)
		// t.Logf("Shared %v", sharedPVCCostCoefficients)
		if err != nil {
			t.Logf("Allocation: Compute: applyPVCsToPods: getPVCCostCoefficients: %s", err)
			continue
		}

		for thisPodKey, coeffComponents := range sharedPVCCostCoefficients {

			pod, ok := podMap[thisPodKey]
			
			// for __unmounted__
			if !ok {
				// Get namespace unmounted pod, as pvc will have a namespace
				// t.Logf("Calling Once")
				pod = getUnmountedPodForNamespace(queryWindow, podMap, pvc.Namespace)
			}

			hrs := utils.ConvertToHours(pod.Window.RunTime())
			gib := pvc.RequestedBytes / 1024 / 1024 / 1024
			cost := pvc.PersistentVolume.CostPerGiBHour * gib * hrs
			byteHours := pvc.RequestedBytes * hrs
			coef := GetCoefficientFromComponents(coeffComponents)
			pvKey := pvc.PersistentVolume.Name

			pod.Allocations[pvKey] = &PersistentVolumeAllocations{
				ByteHours:  (byteHours * coef),
				Cost:       (cost * coef),
				ProviderID: pvc.PersistentVolume.ProviderID,
			}
		}
	}
}

func applyPVUnmountedCosts(queryWindow api.Window, podMap map[PodKey]*PodData, persistentVolumeClaimMap map[PersistentVolumeClaimKey]*PersistentVolumeClaim, t *testing.T) () {

	for _, pvc := range persistentVolumeClaimMap {

		if !pvc.Mounted && pvc.PersistentVolume != nil {

			// Get namespace unmounted pod, as pvc will have a namespace
			pod := getUnmountedPodForNamespace(queryWindow, podMap, pvc.Namespace)

			// Use the Volume Bytes here because pvc bytes could be different,
			// however the pv bytes are what are going to determine cost
			gib := pvc.RequestedBytes / 1024 / 1024 / 1024
			hrs := utils.ConvertToHours(pvc.Window.RunTime())
			cost := pvc.PersistentVolume.CostPerGiBHour * gib * hrs
			pod.Allocations[pvc.PersistentVolume.Name] = &PersistentVolumeAllocations{
				ByteHours: pvc.RequestedBytes * hrs,
				Cost:      cost,
			}
		}
	}
}

// getUnmountedPodForNamespace is as getUnmountedPodForCluster, but keys allocation property pod/namespace field off namespace
// This creates or adds allocations to an unmounted pod in the specified namespace, rather than in __unmounted__
func getUnmountedPodForNamespace(window api.Window, podMap map[PodKey]*PodData, namespace string) *PodData {
	// container := "__unmounted__"
	podName := fmt.Sprintf("%s-unmounted-pvcs", namespace)

	thisPodKey := PodKey{
		Namespace: namespace,
		Pod:       podName,
	}

	// Initialize pod and container if they do not already exist
	thisPod, ok := podMap[thisPodKey]
	if !ok {
		thisPod = &PodData{
			Window: window,
			Allocations: make(map[string]*PersistentVolumeAllocations),
		}

		thisPod.Allocations = make(map[string]*PersistentVolumeAllocations)
		podMap[thisPodKey] = thisPod
	}
	return thisPod
}

func contains(s []PodKey, str PodKey) bool {
    for _, v := range s {
        if v == str {
            return true
        }
    }
    return false
}

// IntervalPoint describes a start or end of a window of time
// Currently, this used in PVC-pod relations to detect/calculate
// coefficients for PV cost when a PVC is shared between pods.
type IntervalPoint struct {
	Time      time.Time
	PointType string
	Key       PodKey
}

// IntervalPoints describes a slice of IntervalPoint structs
type IntervalPoints []IntervalPoint

// Requisite functions for implementing sort.Sort for
// IntervalPointList
func (ips IntervalPoints) Len() int {
	return len(ips)
}

func (ips IntervalPoints) Less(i, j int) bool {
	if ips[i].Time.Equal(ips[j].Time) {
		return ips[i].PointType == "start" && ips[j].PointType == "end"
	}
	return ips[i].Time.Before(ips[j].Time)
}

func (ips IntervalPoints) Swap(i, j int) {
	ips[i], ips[j] = ips[j], ips[i]
}


// NewIntervalPoint creates and returns a new IntervalPoint instance with given parameters.
func NewIntervalPoint(time time.Time, pointType string, key PodKey) IntervalPoint {
	return IntervalPoint{
		Time:      time,
		PointType: pointType,
		Key:       key,
	}
}

// CoefficientComponent is a representative struct holding two fields which describe an interval
// as part of a single number cost coefficient calculation:
// 1. Proportion: The division of cost based on how many pods were running between those points
// 2. Time: The ratio of the time between those points to the total time that pod was running
type CoefficientComponent struct {
	Proportion float64
	Time       float64
}

// getIntervalPointFromWindows takes a map of podKeys to windows
// and returns a sorted list of IntervalPoints representing the
// starts and ends of all those windows.
func GetIntervalPointsFromWindows(windows map[PodKey]api.Window) IntervalPoints {

	var intervals IntervalPoints

	for podKey, podWindow := range windows {

		start := NewIntervalPoint(podWindow.Start, "start", podKey)
		end := NewIntervalPoint(podWindow.End, "end", podKey)

		intervals = append(intervals, []IntervalPoint{start, end}...)

	}

	sort.Sort(intervals)

	return intervals

}

// getPVCCostCoefficients gets a coefficient which represents the scale
// factor that each PVC in a pvcIntervalMap and corresponding slice of
// IntervalPoints intervals uses to calculate a cost for that PVC's PV.
func GetPVCCostCoefficients(intervals IntervalPoints, thisPVC *PersistentVolumeClaim, t *testing.T) (map[PodKey][]CoefficientComponent, error) {
	// pvcCostCoefficientMap has a format such that the individual coefficient
	// components are preserved for testing purposes.
	pvcCostCoefficientMap := make(map[PodKey][]CoefficientComponent)

	pvcWindowDurationMinutes := thisPVC.Window.RunTime()
	
	if pvcWindowDurationMinutes <= 0.0 {
		// Protect against Inf and NaN issues that would be caused by dividing
		// by zero later on.
		return nil, fmt.Errorf("detected PVC with window of zero duration: %s/%s", thisPVC.Namespace, thisPVC.PersistentVolumeClaimName)
	}

	unmountedKey := PodKey{
		Namespace: "__unmounted__",
		Pod: "__unmounted__",
	}

	var void struct{}
	activeKeys := map[PodKey]struct{}{}

	currentTime := thisPVC.Window.Start

	// For each interval i.e. for any time a pod-PVC relation ends or starts...
	for _, point := range intervals {
		// If the current point happens at a later time than the previous point
		if !point.Time.Equal(currentTime) {
			// If there are active keys, attribute one unit of proportion to
			// each active key.
			for key := range activeKeys {
				pvcCostCoefficientMap[key] = append(
					pvcCostCoefficientMap[key],
					CoefficientComponent{
						Time:       point.Time.Sub(currentTime).Minutes() / pvcWindowDurationMinutes,
						Proportion: 1.0 / float64(len(activeKeys)),
					},
				)
			}

			// If there are no active keys attribute all cost to the unmounted pv
			if len(activeKeys) == 0 {
				pvcCostCoefficientMap[unmountedKey] = append(
					pvcCostCoefficientMap[unmountedKey],
					CoefficientComponent{
						Time:       point.Time.Sub(currentTime).Minutes() / pvcWindowDurationMinutes,
						Proportion: 1.0,
					},
				)
			}

		}

		// If the point was a start, increment and track
		if point.PointType == "start" {
			activeKeys[point.Key] = void
		}

		// If the point was an end, decrement and stop tracking
		if point.PointType == "end" {
			delete(activeKeys, point.Key)
		}

		currentTime = point.Time
	}

	// If all pod intervals end before the end of the PVC attribute the remaining cost to unmounted
	if currentTime.Before(thisPVC.Window.End) {

		// PV should be unused for more than the query resolution
		// Not doing this yields weird results as any time difference below the resolution is not accurate without the K8 API.
		resolution, err := utils.ExtractNumericPrefix(Resolution)

		if thisPVC.Window.End.Sub(currentTime).Minutes() > resolution {
			pvcCostCoefficientMap[unmountedKey] = append(
			pvcCostCoefficientMap[unmountedKey],
				CoefficientComponent{
					Time:        thisPVC.Window.End.Sub(currentTime).Minutes() / pvcWindowDurationMinutes,
					Proportion: 1.0,
				},
			)
		} else {
			t.Logf("PVC %v, Pod %v", thisPVC.Window.End, currentTime)
		}
	}
	return pvcCostCoefficientMap, nil
}

// getCoefficientFromComponents takes the components of a PVC-pod PV cost coefficient
// determined by getPVCCostCoefficient and gets the resulting single
// floating point coefficient.
func GetCoefficientFromComponents(coefficientComponents []CoefficientComponent) float64 {

	coefficient := 0.0

	for i := range coefficientComponents {

		proportion := coefficientComponents[i].Proportion
		time := coefficientComponents[i].Time

		coefficient += proportion * time

	}
	return coefficient
}

func TestPVCosts(t *testing.T) {
	apiObj := api.NewAPI()

	// test for more windows
	testCases := []struct {
		name        			string
		window      			string
		aggregate   			string
		accumulate  			string
		includeIdle 			string
		IngestUID				bool
		ConsiderContainerCosts  bool
		CheckUnmountedCosts		bool
	}{
		{
			name:        "Yesterday",
			window:      "24h",
			aggregate:   "pod",
			accumulate:  "true",
			IngestUID: 	 false,
			ConsiderContainerCosts: false,
			CheckUnmountedCosts: false,
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Window Start and End Time
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			// Get Time Duration
			timeMumericVal, _ := utils.ExtractNumericPrefix(tc.window)
			// Assume the minumum unit is an hour
			negativeDuration := time.Duration(timeMumericVal * float64(time.Hour)) * -1
			queryStart := queryEnd.Add(negativeDuration)

			queryWindow := api.Window{
				Start: queryStart,
				End:   queryEnd,
			}
			// We use a 5m resolution [THIS IS THE DEFAULT VALUE IN OPENCOST]
			resolution := 1 * time.Minute

			// Query End Time for all Queries
			endTime := queryEnd.Unix()

			// t.Logf("%v", endTime)

			podMap, podUIDKeyMap := buildPodMap(tc.IngestUID, tc.window, endTime, resolution, queryWindow, t)
			persistentVolumeMap := buildPVMap(tc.window, endTime, resolution, queryWindow, t)
			persistentVolumeClaimMap := buildPVCMap(tc.window, endTime, resolution, queryWindow, persistentVolumeMap, t)
			podPVCMap := buildPodPVCMap(tc.window, endTime, podMap, persistentVolumeMap, persistentVolumeClaimMap, tc.IngestUID, podUIDKeyMap, t)

			// --------------------------------------
			// Apply PVCs to Pod
			// --------------------------------------
			applyPodPVCCosts(queryWindow, podMap, podPVCMap, persistentVolumeClaimMap, t)

			// ----------------------------------------------
			// Unmounted PV Costs
			// ----------------------------------------------
			if tc.CheckUnmountedCosts {
				applyPVUnmountedCosts(queryWindow, podMap, persistentVolumeClaimMap, t)
			}

			
			// ----------------------------------------------
			// Allocation API Data Collection
			// ----------------------------------------------
			// /compute/allocation: PV Costs for all namespaces
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				Accumulate:  tc.accumulate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}

			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// ----------------------------------------------
			// Compare Results with Allocation
			// ----------------------------------------------
			// 5 % Tolerance
			for _, allocationResponseItem := range apiResponse.Data[0] {
				namespace := allocationResponseItem.Properties.Namespace
				pod := allocationResponseItem.Properties.Pod

				
				if strings.Contains(pod, "pvcs") {
					t.Logf("Unmounted Pod")
				}

				// container := allocationResponseItem.Properties.Container

				podKey := PodKey{
					Namespace: namespace,
					Pod:       pod,
				}

				// podItem := podMap[podKey]

				

				// Get Pods
				if tc.IngestUID {
					podKeys := podUIDKeyMap[podKey]
					
					podMap[podKey] = &PodData{
						Pod: pod,
						Namespace: namespace,
						Allocations: make(map[string]*PersistentVolumeAllocations),
					}
					
					podItem := podMap[podKey]
				// // In the case of unmounted pods
				// if container == "__unmounted__" {
				// 	podKeys = []prometheus.PodKey{podUIDKey}
				// 	continue
				// }

				// podData1 := &prometheus.PodData{}
				// podData1.Containers = make(map[string]map[string]*prometheus.PVAllocations)
					for _, key := range podKeys {
						thisPod := podMap[key]
						// if thisPod.Pod == "prometheus-prometheus-kube-prometheus-prometheus-0" {
						// 	t.Logf("PodKeys Loop%v", thisPod.Pod)
						// 	t.Logf("PodKey %v", podKeys)
						// 		// t.Logf("PV %v", pv)
						// 		// t.Logf("ByteHours: %v", thisPod.Containers[container][pv].ByteHours)
						// }
						if thisPod == nil {
							continue
						}
						// if _, ok := podData1.Containers[container]; !ok {
						// 	podData1.Containers[container] = thisPod.Containers[container]
						// 	continue
						// }
						// if container == "__unmounted__" {
						// 	continue
						// }
						for pv, pvinfo := range thisPod.Allocations {
							podinfo, ok := podItem.Allocations[pv]
							// if thisPod.Pod == "prometheus-prometheus-kube-prometheus-prometheus-0" {
							// 	t.Logf("%v", thisPod.Pod)
							// 	t.Logf("PV %v", pv)
							// 	t.Logf("ByteHours: %v", thisPod.Containers[container][pv].ByteHours)
							// }
							if !ok {
								podItem.Allocations[pv] = &PersistentVolumeAllocations{
									ByteHours: pvinfo.ByteHours,
									Cost: pvinfo.Cost,
									ProviderID: pvinfo.ProviderID,
								}
								continue
							}
							podinfo.ByteHours += pvinfo.ByteHours
							podinfo.Cost += pvinfo.Cost
						}
						
					}
				}

				podItem, ok := podMap[podKey]
				
				// Maybe for Unmounted Pods
				if !ok {
					t.Logf("PodKey %v missing from Prometheus", podKey)
					continue
				}
				// if !ok {
				// 	if container == "__unmounted__" { // If promethues starts recognising unmounted pods, remove this. Temporary Fix
				// 		t.Logf("[Skipping] Unmounted PVs not supported")
				// 		continue
				// 	}
				// 	t.Errorf("Pod Information Missing from API")
				// 	continue
				// }

				// Get Containers
				// containerPVs, ok := podData1.Containers[container]
				// if !ok {
				// 	t.Errorf("Container Information Missing from API")
				// }

				if allocationResponseItem.PersistentVolumes != nil {
					// Loop Over Persistent Volume Claims
					if len(allocationResponseItem.PersistentVolumes) != 0 {
						t.Logf("Pod: %v", pod)
						t.Logf("Pod Runtime: %v", allocationResponseItem.Minutes)
					}

					for allocPVName, allocPV := range allocationResponseItem.PersistentVolumes {
						allocProviderID := allocPV.ProviderID
						allocByteHours := allocPV.ByteHours
						allocCost := allocPV.Cost

						// Get PV Name
						// allocPVName = cluster=default-cluster:name=csi-7da248e4-1143-4c64-ab24-3ab1ba178f9
						re := regexp.MustCompile(`name=([^:]+)`)
						allocPVName := re.FindStringSubmatch(allocPVName)[1]
						
						// t.Logf("%v", allocPVName)
						allocPVItem, ok := podItem.Allocations[allocPVName]
						if !ok {
							continue
						}

						if allocPVItem.ProviderID != allocProviderID {
							t.Errorf("Provider IDs don't match for the same Pod")
							continue
						}

						t.Logf("  - Persistent Volume Name: %v", allocPVName)

						// Compare ByteHours
						withinRange, diff_percent := utils.AreWithinPercentage(allocPVItem.ByteHours, allocByteHours, Tolerance)
						if withinRange {
							t.Logf("      - ByteHours[Pass]: ~%0.2f", allocByteHours)
						} else {
							t.Errorf("      - ByteHours[Fail]: DifferencePercent: %0.2f, Prom Results: %0.4f, API Results: %0.4f", diff_percent, allocPVItem.ByteHours, allocByteHours)
						}
						// Compare Cost
						
						if allocCost < NegligibleCost {
							continue
						}
						withinRange, diff_percent = utils.AreWithinPercentage(allocPVItem.Cost, allocCost, Tolerance)
						if withinRange {
							t.Logf("      - Cost[Pass]: ~%0.2f", allocCost)
						} else {
							t.Errorf("      - Cost[Fail]: DifferencePercent: %0.2f, Prom Results: %0.4f, API Results: %0.4f", diff_percent,allocPVItem.Cost, allocCost)
						}
					}
				}
				
			}
		})
	}
}