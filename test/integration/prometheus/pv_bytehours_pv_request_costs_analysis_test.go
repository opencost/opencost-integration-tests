package prometheus

// Description: Checks all PV related Costs
// - PVBytes
// - PVBytesHours
// - PVBytesRequestAverage

// Testing Methodology
// To Do

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	// "github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.05
const KiB = 1024.0
const MiB = 1024.0 * KiB
const GiB = 1024.0 * MiB
const TiB = 1024.0 * GiB
const PiB = 1024.0 * TiB
const PV_USAGE_SANITY_LIMIT_BYTES = 10.0 * PiB

func TestPVCosts(t *testing.T) {
	apiObj := api.NewAPI()

	// test for more windows
	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate string
	}{
		{
			name:       "Yesterday",
			window:     "24h",
			aggregate:  "pod,container",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// ----------------------------------------------
			// Allocation API Data Collection
			// ----------------------------------------------
			// /compute/allocation: PV Costs for all namespaces
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:     tc.window,
				Aggregate:  tc.aggregate,
				Accumulate: tc.accumulate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// ----------------------------------------------
			// Prometheus Data Collection
			// ----------------------------------------------
			client := prometheus.NewClient()

			// Loop over namespaces
			// for namespace, allocationResponseItem := range apiResponse.Data[0] {

			// Window Start and End Time
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			queryStart := queryEnd.Add(-24 * time.Hour)
			window24h := api.Window{
				Start: queryStart,
				End:   queryEnd,
			}
			// We use a 5m resolution [THIS IS THE DEFAULT VALUE IN OPENCOST]
			resolution := 5 * time.Minute

			// Query End Time for all Queries
			endTime := queryEnd.Unix()

			// ----------------------------------------------
			// Metric: PVActiveMins
			// Description: Get Alive Time for the Persistent Volume

			// avg(kube_persistentvolume_capacity_bytes{%s}) by (%s, persistentvolume)[%s:%dm]`
			// ----------------------------------------------
			promPVRunTime := prometheus.PrometheusInput{}
			promPVRunTime.Metric = "kube_persistentvolume_capacity_bytes"
			// promPVRunTime.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPVRunTime.AggregateBy = []string{"persistentvolume"}
			promPVRunTime.Function = []string{"avg"}
			promPVRunTime.AggregateWindow = tc.window
			promPVRunTime.AggregateResolution = "5m"
			promPVRunTime.Time = &endTime

			PVRunTime, err := client.RunPromQLQuery(promPVRunTime)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PVBytes
			// Description: Get PersistentVolume Capacity

			// avg(avg_over_time(kube_persistentvolume_capacity_bytes{%s}[%s])) 
			// by 
			// (persistentvolume, %s)`
			// ----------------------------------------------
			promPVBytes := prometheus.PrometheusInput{}
			promPVBytes.Metric = "kube_persistentvolume_capacity_bytes"
			// promPVBytes.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPVBytes.AggregateBy = []string{"persistentvolume"}
			promPVBytes.Function = []string{"avg_over_time", "avg"}
			promPVBytes.QueryWindow = tc.window
			promPVBytes.Time = &endTime

			PVBytes, err := client.RunPromQLQuery(promPVBytes)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PVCostPerGibHour
			// Description: Get Cost for Every Byte Used in an Hour in GigaBytes

			// avg(avg_over_time(pv_hourly_cost{%s}[%s]))
			// by 
			// (%s, persistentvolume, volumename, provider_id)
			// ----------------------------------------------
			promCostPerGiBHour := prometheus.PrometheusInput{}
			promCostPerGiBHour.Metric = "pv_hourly_cost"
			// promCostPerGiBHour.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promCostPerGiBHour.AggregateBy = []string{"persistentvolume", "volumename", "provider_id"}
			promCostPerGiBHour.Function = []string{"avg_over_time", "avg"}
			promCostPerGiBHour.QueryWindow = tc.window
			promCostPerGiBHour.Time = &endTime

			PVCostPerGiBHour, err := client.RunPromQLQuery(promCostPerGiBHour)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PVMeta
			// Description: Persistent Volume Information

			// avg(avg_over_time(kubecost_pv_info{%s}[%s])) 
			// by 
			// (%s, storageclass, persistentvolume, provider_id)
			// ----------------------------------------------
			promPVMeta := prometheus.PrometheusInput{}
			promPVMeta.Metric = "kubecost_pv_info"
			// promPVMeta.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPVMeta.AggregateBy = []string{"storageclass", "persistentvolume", "provider_id"}
			promPVMeta.Function = []string{"avg_over_time", "avg"}
			promPVMeta.QueryWindow = tc.window
			promPVMeta.Time = &endTime

			PVMeta, err := client.RunPromQLQuery(promPVMeta)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PVCInfo
			// Description: Persistent Volume Claim Information

			// avg(kube_persistentvolumeclaim_info{volumename != "", %s})
			// by 
			// (persistentvolumeclaim, storageclass, volumename, namespace, %s)[%s:%dm]
			// ----------------------------------------------
			promPVCInfo := prometheus.PrometheusInput{}
			promPVCInfo.Metric = "kube_persistentvolumeclaim_info"
			// promPVCInfo.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPVCInfo.IgnoreFilters = map[string][]string{
				"volumename": {""},
			}
			promPVCInfo.AggregateBy = []string{"persistentvolumeclaim", "storageclass", "volumename", "namespace"}
			promPVCInfo.Function = []string{"avg"}
			promPVCInfo.AggregateWindow = tc.window
			promPVCInfo.AggregateResolution = "5m"
			promPVCInfo.Time = &endTime

			PVCInfo, err := client.RunPromQLQuery(promPVCInfo)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PVCRequestedBytes
			// Description: Persistent Volume Claim Requested Bytes

			// avg(avg_over_time(kube_persistentvolumeclaim_resource_requests_storage_bytes{%s}[%s]))
			// by 
			// (persistentvolumeclaim, namespace, %s)
			// ----------------------------------------------
			promPVCRequestedBytes := prometheus.PrometheusInput{}
			promPVCRequestedBytes.Metric = "kube_persistentvolumeclaim_resource_requests_storage_bytes"
			// promPVCRequestedBytes.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPVCRequestedBytes.AggregateBy = []string{"persistentvolumeclaim", "namespace"}
			promPVCRequestedBytes.Function = []string{"avg_over_time", "avg"}
			promPVCRequestedBytes.QueryWindow = tc.window
			promPVCRequestedBytes.Time = &endTime

			PVCRequestedBytes, err := client.RunPromQLQuery(promPVCRequestedBytes)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// ----------------------------------------------
			// Metric: PodPVCAllocation
			// Description: Pod Persistent Volume Claim Allocation

			// avg(avg_over_time(pod_pvc_allocation{%s}[%s]))
			// by 
			// (persistentvolume, persistentvolumeclaim, pod, namespace, %s)
			// ----------------------------------------------
			promPodPVCAllocation := prometheus.PrometheusInput{}
			promPodPVCAllocation.Metric = "pod_pvc_allocation"
			// promPodPVCAllocation.Filters = map[string]string{
			// 	"namespace": namespace,
			// }
			promPodPVCAllocation.AggregateBy = []string{"persistentvolume", "persistentvolumeclaim", "pod", "namespace"}
			promPodPVCAllocation.Function = []string{"avg_over_time", "avg"}
			promPodPVCAllocation.QueryWindow = tc.window
			promPodPVCAllocation.Time = &endTime

			PodPVCAllocation, err := client.RunPromQLQuery(promPodPVCAllocation)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}



			// Phase - II
			// Create and Populate a PersistentVolume Map
			// Create and Populate a PersistentVolumeClaim Map
			// Use Allocation, and Pod data to get coefficient factor

			// --------------------------------------
			// Populate PersistentVolume Map
			// --------------------------------------
			
			PersistentVolumeMap := make(map[string]*prometheus.PersistentVolume)

			// Start and End Times for a PV
			for _, promPVRunTimeItem := range PVRunTime.Data.Result {
				persistentVolumeName := promPVRunTimeItem.Metric.PersistentVolume
				s, e := prometheus.CalculateStartAndEnd(promPVRunTimeItem.Values, resolution, window24h)
				PersistentVolumeMap[persistentVolumeName] = &prometheus.PersistentVolume{
					Name: persistentVolumeName,
					Start: s,
					End: e,
					CostPerGiBHour: 0.0,
					ProviderID: "",
				}
			}

			// CostPerGiBHour for a PV
			for _, promCostPerGiBHourItem := range PVCostPerGiBHour.Data.Result {
				persistentVolumeName := promCostPerGiBHourItem.Metric.PersistentVolume
				// providerId := promCostPerGiBHourItem.Metric.ProviderID
				
				PVItem, ok := PersistentVolumeMap[persistentVolumeName]
				if !ok {
					t.Errorf("PersistentVolume %s missing from kube_persistentvolume_capacity_bytes", persistentVolumeName)
					continue
				}
				PVItem.CostPerGiBHour = promCostPerGiBHourItem.Value.Value
			}

			// only add metadata for disks that exist in the other metrics
			for _, promPVMetaItem := range PVMeta.Data.Result {
				persistentVolumeName := promPVMetaItem.Metric.PersistentVolume
				providerId := promPVMetaItem.Metric.ProviderID
				
				if PVItem, ok := PersistentVolumeMap[persistentVolumeName]; ok {
					if providerId != "" {
						PVItem.ProviderID = providerId
					}
				}
			}

			// Add PVBytes for PV
			for _, promPVBytesItem := range PVBytes.Data.Result{
				persistentVolumeName := promPVBytesItem.Metric.PersistentVolume
				// providerId := promPVBytesItem.Metric.ProviderID
				
				PVItem, ok := PersistentVolumeMap[persistentVolumeName]
				if !ok {
					t.Errorf("PersistentVolume %s missing from kube_persistentvolume_capacity_bytes", persistentVolumeName)
					continue
				}
				PVItem.PVBytes = promPVBytesItem.Value.Value
				
				// PV usage exceeds sanity limit
				if PVItem.PVBytes > PV_USAGE_SANITY_LIMIT_BYTES {
					t.Logf("PV usage exceeds sanity limit, clamping to zero for %s", persistentVolumeName)
					PVItem.PVBytes = 0.0
				}
			}

			// --------------------------------------
			// Populate PersistentVolumeClaim Map
			// --------------------------------------

			type PersistentVolumeClaimKey struct {
				Namespace					string
				PersistentVolumeClaimName	string
			}

			
			// Map Key is Namespace
			PersistentVolumeClaimMap := make(map[PersistentVolumeClaimKey]*prometheus.PersistentVolumeClaim)

			// Get PVC Information
			for _, promPVCInfoItem := range PVCInfo.Data.Result {
				persistentVolumeName := promPVCInfoItem.Metric.PersistentVolume
				persistentVolumeClaimName := promPVCInfoItem.Metric.PersistentVolumeClaim
				storageClass := promPVCInfoItem.Metric.StorageClass
				namespace := promPVCInfoItem.Metric.Namespace
				
				if namespace == "" || persistentVolumeClaimName == "" || persistentVolumeName == "" || storageClass == "" {
					t.Logf("CostModel.ComputeAllocation: pvc info query result missing field")
					continue
				}

				PVItem, ok := PersistentVolumeMap[persistentVolumeName]
				if !ok {
					continue
				}

				PVItem.StorageClass = storageClass

				// Get Start and End time for Persistent Volume Claim
				s, e := prometheus.CalculateStartAndEnd(promPVCInfoItem.Values, resolution, window24h)

				// Create a PersistentVolume Index and link the PersistentVolume Information
				persistentVolumeClaimKey := PersistentVolumeClaimKey{
					Namespace: namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
				}

				PersistentVolumeClaimMap[persistentVolumeClaimKey] = &prometheus.PersistentVolumeClaim{
					Namespace: namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
					PersistentVolume: PVItem,
					Start: s,
					End: e,
				}
			}	
			
			// Add PVCRequestedBytes
			for _, promPVCRequestedBytesItem := range PVCRequestedBytes.Data.Result {
				persistentVolumeClaimName := promPVCRequestedBytesItem.Metric.PersistentVolumeClaim
				namespace := promPVCRequestedBytesItem.Metric.Namespace

				persistentVolumeClaimKey := PersistentVolumeClaimKey{
					Namespace: namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
				}
				
				PersistentVolumeClaimItem, ok := PersistentVolumeClaimMap[persistentVolumeClaimKey]
				if !ok {
					continue
				}

				// Add Requested Bytes Information
				PersistentVolumeClaimItem.RequestedBytes = promPVCRequestedBytesItem.Value.Value
			}

			// --------------------------------------
			// Build Pod Map
			// --------------------------------------
			
			// Query all running pod information
			// avg(kube_pod_container_status_running{} != 0)
			// by
			// (container, pod, namespace, node)[24h:5m]

			// Q) != 0 is not necessary I suppose?
			promPodInfoInput := prometheus.PrometheusInput{}

			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.Filters = map[string]string{
			}
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "uid"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = tc.window
			promPodInfoInput.AggregateResolution = "5m"
			promPodInfoInput.Time = &endTime

			podInfo, err := client.RunPromQLQuery(promPodInfoInput)

			type PVAllocations struct {
				ByteHours		float64
				Cost			float64
				ProviderID		string
			}

			type PodData struct {
				Pod        	string
				Namespace  	string
				Start      	time.Time
				End        	time.Time
				Minutes    	float64
				Containers map[string]*PVAllocations
				
			}

			podMap := make(map[prometheus.PodKey]*PodData)
			podUIDKeyMap := make(map[prometheus.PodKey][]prometheus.PodKey)

			for _, podInfoResponseItem := range podInfo.Data.Result {

				namespace := podInfoResponseItem.Metric.Namespace
				pod := podInfoResponseItem.Metric.Pod
				container := podInfoResponseItem.Metric.Container
				uid := podInfoResponseItem.Metric.UID
				
				// This is to account for pod replicas
				if uid == "" {
					continue
				}

				podKey := prometheus.PodKey{
					Namespace: namespace,
					Pod: pod,
				}

				newPodKey := prometheus.PodKey{
					Namespace: namespace,
					Pod: pod,
					UID: uid,
				}

				podUIDKeyMap[podKey] = append(podUIDKeyMap[podKey], newPodKey)

				s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)

				if thisPod, ok := podMap[newPodKey]; ok {
					// Expand Pod Run Intervals based as an integration of all UIDs
					if s.Before(thisPod.Start) {
						thisPod.Start = s
					}
					if e.After(thisPod.End) {
						thisPod.End = e
					}
				
				} else {
					podMap[newPodKey] = &PodData{
						// Key:        	newPodKey,
						Namespace:  	namespace,
						Start:      	s,
						End:        	e,
						Minutes:    	e.Sub(s).Minutes(),
						Containers:		make(map[string]map[string]PVAllocations),
					}
					
				}
				podMap[newPodKey].Containers[container] = make(map[string]*PVAllocations)

			}

			// --------------------------------------
			// Populate Pod to PersistentVolumeClaim Map
			// --------------------------------------

			// Index is a Pod
			podPVCMap := make(map[string][]*prometheus.PersistentVolumeClaim)

			for _, podPVCAllocationItem := range PodPVCAllocation {
				
				namespace := podPVCAllocationItem.Namespace
				pod := podPVCAllocationItem.Pod
				persistentVolumeName := podPVCAllocationItem.PersistentVolume
				persistentVolumeClaimName := podPVCAllocationItem.PersistentVolumeClaim

				if namespace == "" || pod == "" || persistentVolumeName == "" || persistentVolumeClaimName == "" {
					t.Logf("CostModel.ComputeAllocation: pvc allocation query result missing field")
					continue
				}

				podKey := prometheus.PodKey{
					Namespace: namespace,
					Pod: pod,
				}

				persistentVolumeClaimKey := PersistentVolumeClaimKey{
					Namespace: namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName, 
				}

				for _, key := range podUIDKeyMap {
					// Add Error Checking Later
					pvc, ok := PersistentVolumeClaimMap[persistentVolumeClaimKey]
					pvc.Mounted = true
					podPVCMap[key] = append(podPVCMap[key], pvc)
				}
			}

			// --------------------------------------
			// Apply PVCs to Pod
			// --------------------------------------
			

			// For each persistent volume
			// Attach the pod along with a modified run time based on the persistent volume.
			// This interval is the intersection of the persistentvolume alive time and pod alive time intervals

			pvcPodWindowMap := make(map[PersistentVolumeClaimKey]map[PodKey]api.Window)

			for thisPodKey, thisPod := range podMap {
				// Get all persistent volume claims made by a namespace that a pod belongs to
				if pvcs, ok := podPVCMap[thisPodKey]; ok {
					for _, thisPVC := range pvcs {

						// Try to limit the usage interval of persistentclaim for this pod to the pod's window size
						s, e := thisPod.Start, thisPod.End
						if thisPVC.Start.After(thisPod.Start) {
							s = thisPVC.Start
						}
						if thisPVC.End.Before(thisPod.End) {
							e = thisPVC.End
						}

						thisPVCKey := PersistentVolumeClaimKey{
							Namespace: thisPVC.Namespace,
							PersistentVolumeClaimName: thisPVC.PersistentVolumeClaimName,
						}

						if pvcPodWindowMap[thisPVCKey] == nil {
							pvcPodWindowMap[thisPVCKey] = make(map[prometheus.PodKey]api.Window)
						}
						pvcPodWindowMap[thisPVCKey][thisPodKey] = api.Window{
							Start: s,
							End: e,
						}
					}
				}
			}

			for thisPVCKey, pvcPodWindowMap := range pvcPodWindowMap {

				// Sort the pod intervals in ascending order
				intervals := prometheus.GetIntervalPointsFromWindows(pvcPodWindowMap)

				// Check for errors later
				pvc, _ := PersistentVolumeClaimMap[thisPVCKey]

				sharedPVCCostCoefficients, err := prometheus.GetPVCCostCoefficients(intervals, pvc)
				if err != nil {
					t.Logf("Allocation: Compute: applyPVCsToPods: getPVCCostCoefficients: %s", err)
					continue
				}
			
				for thisPodKey, coeffComponents := range sharedPVCCostCoefficients {
					
					pod, ok2 := podMap[thisPodKey]

					for _, alloc := range pod.Containers {
						s, e := pod.Start, pod.End

						minutes := e.Sub(s).Minutes()
						hrs := minutes / 60.0

						gib := pvc.RequestedBytes / 1024 / 1024 / 1024
						cost := pvc.PersistentVolume.CostPerGiBHour * gib * hrs
						byteHours := pvc.RequestedBytes * hrs
						coef := prometheus.GetCoefficientFromComponents(coeffComponents)
						pvKey := pvc.PersistentVolume.Name

						// Both Cost and byteHours should be multiplied by the coef and divided by count
						// so that if all allocations with a given pv key are summed the result of those
						// would be equal to the values of the original pv
						count := float64(len(pod.Containers))
						alloc.PVAllocations[pvKey] = &prometheus.PVAllocation{
							ByteHours:  byteHours * coef / count,
							Cost:       cost * coef / count,
							ProviderID: pvc.PersistentVolume.ProviderID,
						}
					}
				}
			}

			//----------------------------------------------
			// Compare Results with Allocation
			// ----------------------------------------------
			// Loop over what?
			// 5 % Tolerance
			// withinRange, diff_percent := utils.AreWithinPercentage(nsPVBytes, allocationResponseItem.PVBytes, tolerance)
			// if withinRange {
			// 	t.Logf("    - PVBytes[Pass]: ~%.2f", nsPVBytes)
			// } else {
			// 	t.Errorf("    - PVBytes[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsPVBytes, allocationResponseItem.PVBytes)
			// }
			// withinRange, diff_percent = utils.AreWithinPercentage(nsPVBytesHours, allocationResponseItem.PVByteHours, tolerance)
			// if withinRange {
			// 	t.Logf("    - PVByteHours[Pass]: ~%.2f", nsPVBytesHours)
			// } else {
			// 	t.Errorf("    - PVByteHours[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsPVBytesHours, allocationResponseItem.PVByteHours)
			// }
		})
	}
}
