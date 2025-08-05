package prometheus

// Description: Checks all PV related Costs
// - PVBytes
// - PVBytesHours
// - PVBytesRequestAverage

// Testing Methodology: Step by Step Information
//     - Get Persistent Volume RunTimes
//	   - Get Persistent Volume Information
//	   - Get Persistent Volume Claim Information
//	   - Get Pod Information and create a PodMap
// 	   - Create PersistentVolume and PersistentVolumeClaimMap
//	   - Compute Cost of PVC usage for each pod based on its running interval in the pods lifecycle

import (
	"fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"regexp"
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
		name        string
		window      string
		aggregate   string
		accumulate  string
		includeIdle string
	}{
		{
			name:        "Yesterday",
			window:      "24h",
			aggregate:   "pod,container",
			accumulate:  "False",
			includeIdle: "True",
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
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				Accumulate:  tc.accumulate,
				IncludeIdle: tc.includeIdle,
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
					Name:           persistentVolumeName,
					Start:          s,
					End:            e,
					CostPerGiBHour: 0.0,
					ProviderID:     "",
				}
			}

			// CostPerGiBHour for a PV
			for _, promCostPerGiBHourItem := range PVCostPerGiBHour.Data.Result {
				persistentVolumeName := promCostPerGiBHourItem.Metric.PersistentVolume
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
			for _, promPVBytesItem := range PVBytes.Data.Result {
				persistentVolumeName := promPVBytesItem.Metric.PersistentVolume
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
				Namespace                 string
				PersistentVolumeClaimName string
			}

			// Map Key is Namespace
			PersistentVolumeClaimMap := make(map[PersistentVolumeClaimKey]*prometheus.PersistentVolumeClaim)

			// Get PVC Information
			for _, promPVCInfoItem := range PVCInfo.Data.Result {
				persistentVolumeName := promPVCInfoItem.Metric.VolumeName
				persistentVolumeClaimName := promPVCInfoItem.Metric.PersistentVolumeClaim
				storageClass := promPVCInfoItem.Metric.StorageClass
				namespace := promPVCInfoItem.Metric.Namespace

				if namespace == "" || persistentVolumeClaimName == "" || persistentVolumeName == "" || storageClass == "" {
					t.Logf("PV Test: pvc info query result missing field")
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
					Namespace:                 namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
				}

				PersistentVolumeClaimMap[persistentVolumeClaimKey] = &prometheus.PersistentVolumeClaim{
					Namespace:                 namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
					PersistentVolume:          PVItem,
					Start:                     s,
					End:                       e,
				}
			}

			// Add PVCRequestedBytes
			for _, promPVCRequestedBytesItem := range PVCRequestedBytes.Data.Result {
				persistentVolumeClaimName := promPVCRequestedBytesItem.Metric.PersistentVolumeClaim
				namespace := promPVCRequestedBytesItem.Metric.Namespace

				persistentVolumeClaimKey := PersistentVolumeClaimKey{
					Namespace:                 namespace,
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
			// (container, pod, namespace, uid)[24h:5m]

			// Q) != 0 is not necessary I suppose?
			promPodInfoInput := prometheus.PrometheusInput{}

			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.Filters = map[string]string{}
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "uid"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = tc.window
			promPodInfoInput.AggregateResolution = "5m"
			promPodInfoInput.Time = &endTime

			podInfo, err := client.RunPromQLQuery(promPodInfoInput)

			podMap := make(map[prometheus.PodKey]*prometheus.PodData)
			podUIDKeyMap := make(map[prometheus.PodKey][]prometheus.PodKey)

			for _, podInfoResponseItem := range podInfo.Data.Result {

				namespace := podInfoResponseItem.Metric.Namespace
				pod := podInfoResponseItem.Metric.Pod
				container := podInfoResponseItem.Metric.Container
				uid := podInfoResponseItem.Metric.UID

				podKey := prometheus.PodKey{
					Namespace: namespace,
					Pod:       pod,
				}

				// This is to account for pod replicas
				if uid == "" {
					t.Logf("Query Result missing UID for Pod %s", pod)
				} else {
					newPodKey := prometheus.PodKey{
						Namespace: namespace,
						Pod:       pod,
						UID:       uid,
					}
					podUIDKeyMap[podKey] = append(podUIDKeyMap[podKey], newPodKey)
					podKey = newPodKey

				}

				s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)

				if s.IsZero() || e.IsZero() {
					continue
				}

				if thisPod, ok := podMap[podKey]; ok {
					// Expand Pod Run Intervals based as an integration of all UIDs
					if s.Before(thisPod.Start) {
						thisPod.Start = s
					}
					if e.After(thisPod.End) {
						thisPod.End = e
					}

				} else {
					podMap[podKey] = &prometheus.PodData{
						// Key:        	newPodKey,
						Namespace:  namespace,
						Start:      s,
						End:        e,
						Minutes:    e.Sub(s).Minutes(),
						Containers: make(map[string]map[string]*prometheus.PVAllocations),
					}

				}
				podMap[podKey].Containers[container] = make(map[string]*prometheus.PVAllocations)
			}

			// --------------------------------------
			// Populate Pod to PersistentVolumeClaim Map
			// --------------------------------------

			// Index is a Pod
			podPVCMap := make(map[prometheus.PodKey][]*prometheus.PersistentVolumeClaim)

			for _, podPVCAllocationItem := range PodPVCAllocation.Data.Result {

				namespace := podPVCAllocationItem.Metric.Namespace
				pod := podPVCAllocationItem.Metric.Pod
				persistentVolumeName := podPVCAllocationItem.Metric.PersistentVolume
				persistentVolumeClaimName := podPVCAllocationItem.Metric.PersistentVolumeClaim

				if namespace == "" || pod == "" || persistentVolumeName == "" || persistentVolumeClaimName == "" {
					t.Logf("PV Test: pvc allocation query result missing field")
					continue
				}

				podKey := prometheus.PodKey{
					Namespace: namespace,
					Pod:       pod,
				}

				persistentVolumeClaimKey := PersistentVolumeClaimKey{
					Namespace:                 namespace,
					PersistentVolumeClaimName: persistentVolumeClaimName,
				}

				for _, key := range podUIDKeyMap[podKey] {
					// Add Error Checking Later

					if _, ok := PersistentVolumeMap[persistentVolumeName]; !ok {
						t.Logf("PV Test: pv missing for pvc allocation query result: %s", persistentVolumeName)
					}

					pvc, ok := PersistentVolumeClaimMap[persistentVolumeClaimKey]
					if !ok {
						t.Logf("PV Test: pvc missing for from PVC alloctions prom query: %s", persistentVolumeClaimKey)
						continue
					}

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

			pvcPodWindowMap := make(map[PersistentVolumeClaimKey]map[prometheus.PodKey]api.Window)

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
							Namespace:                 thisPVC.Namespace,
							PersistentVolumeClaimName: thisPVC.PersistentVolumeClaimName,
						}

						if pvcPodWindowMap[thisPVCKey] == nil {
							pvcPodWindowMap[thisPVCKey] = make(map[prometheus.PodKey]api.Window)
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
				intervals := prometheus.GetIntervalPointsFromWindows(pvcPodWindowMap)

				// Check for errors later
				pvc, _ := PersistentVolumeClaimMap[thisPVCKey]

				sharedPVCCostCoefficients, err := prometheus.GetPVCCostCoefficients(intervals, pvc)
				t.Logf("Shared %v", sharedPVCCostCoefficients)
				if err != nil {
					t.Logf("Allocation: Compute: applyPVCsToPods: getPVCCostCoefficients: %s", err)
					continue
				}

				for thisPodKey, coeffComponents := range sharedPVCCostCoefficients {

					pod, ok := podMap[thisPodKey]

					if !ok || len(pod.Containers) == 0 {
						// Get namespace unmounted pod, as pvc will have a namespace
						window := api.Window{
							Start: queryStart,
							End:   queryEnd,
						}
						pod = getUnmountedPodForNamespace(window, podMap, pvc.Namespace)
					}

					// Alloc is pvs, the map of persistent volumes
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
						alloc[pvKey] = &prometheus.PVAllocations{
							ByteHours:  byteHours * coef / count,
							Cost:       cost * coef / count,
							ProviderID: pvc.PersistentVolume.ProviderID,
						}
					}
				}
			}

			// ----------------------------------------------
			// Unmounted PV Costs
			// ----------------------------------------------

			for _, pvc := range PersistentVolumeClaimMap {
				if !pvc.Mounted && pvc.PersistentVolume != nil {

					// Get namespace unmounted pod, as pvc will have a namespace
					window := api.Window{
						Start: queryStart,
						End:   queryEnd,
					}

					pod := getUnmountedPodForNamespace(window, podMap, pvc.Namespace)

					// Use the Volume Bytes here because pvc bytes could be different,
					// however the pv bytes are what are going to determine cost
					gib := pvc.RequestedBytes / 1024 / 1024 / 1024
					hrs := pvc.End.Sub(pvc.Start).Minutes() / 60
					cost := pvc.PersistentVolume.CostPerGiBHour * gib * hrs
					pod.Containers["__unmounted__"][pvc.PersistentVolume.Name] = &prometheus.PVAllocations{
						ByteHours: pvc.RequestedBytes * hrs,
						Cost:      cost,
					}
				}
			}

			// ----------------------------------------------
			// Compare Results with Allocation
			// ----------------------------------------------
			// 5 % Tolerance
			for _, allocationResponseItem := range apiResponse.Data[0] {
				namespace := allocationResponseItem.Properties.Namespace
				pod := allocationResponseItem.Properties.Pod
				container := allocationResponseItem.Properties.Container

				podUIDKey := prometheus.PodKey{
					Namespace: namespace,
					Pod:       pod,
				}

				// Get Pods
				podKeys := podUIDKeyMap[podUIDKey]

				// In the case of unmounted pods
				if container == "__unmounted__" {
					podKeys = []prometheus.PodKey{podUIDKey}
				}

				for _, podKey := range podKeys {

					podData, ok := podMap[podKey]
					if !ok {
						if container == "__unmounted__" { // If promethues starts recognising unmounted pods, remove this. Temporary Fix
							t.Logf("[Skipping] Unmounted PVs not supported")
							continue
						}
						t.Errorf("Pod Information Missing from API")
						continue
					}

					// Get Containers
					containerPVs, ok := podData.Containers[container]
					if !ok {
						t.Errorf("Container Information Missing from API")
					}

					if allocationResponseItem.PersistentVolumes != nil {
						// Loop Over Persistent Volume Claims
						if len(allocationResponseItem.PersistentVolumes) != 0 {
							t.Logf("Container Name: %v, Pod: %v, Pod UID: %v", container, pod, podKey.UID)
						}
						for allocPVName, allocPV := range allocationResponseItem.PersistentVolumes {
							allocProviderID := allocPV.ProviderID
							allocByteHours := allocPV.ByteHours
							allocCost := allocPV.Cost

							// Get PV Name
							// allocPVName = cluster=default-cluster:name=csi-7da248e4-1143-4c64-ab24-3ab1ba178f9
							re := regexp.MustCompile(`name=([^:]+)`)
							allocPVName := re.FindStringSubmatch(allocPVName)[1]
							
							_, ok := containerPVs[allocPVName]
							if !ok {
								continue
							}

							if containerPVs[allocPVName].ProviderID != allocProviderID {
								t.Errorf("Provider IDs don't match for the same Pod")
								continue
							}

							t.Logf("  - Persistent Volume Name: %v", allocPVName)
							// Compare ByteHours
							withinRange, diff_percent := utils.AreWithinPercentage(containerPVs[allocPVName].ByteHours, allocByteHours, tolerance)
							if withinRange {
								t.Logf("      - ByteHours[Pass]: ~%0.2f", allocByteHours)
							} else {
								t.Errorf("      - ByteHours[Fail]: DifferencePercent: %0.2f, Prom Results: %0.2f, API Results: %0.2f", diff_percent, containerPVs[allocPVName].ByteHours, allocByteHours)
							}
							// Compare Cost
							withinRange, diff_percent = utils.AreWithinPercentage(containerPVs[allocPVName].Cost, allocCost, tolerance)
							if withinRange {
								t.Logf("      - Cost[Pass]: ~%0.2f", allocCost)
							} else {
								t.Errorf("      - Cost[Fail]: DifferencePercent: %0.2f, Prom Results: %0.2f, API Results: %0.2f", diff_percent, containerPVs[allocPVName].Cost, allocCost)
							}
						}
					}
				}
			}
		})
	}
}

// getUnmountedPodForNamespace is as getUnmountedPodForCluster, but keys allocation property pod/namespace field off namespace
// This creates or adds allocations to an unmounted pod in the specified namespace, rather than in __unmounted__
func getUnmountedPodForNamespace(window api.Window, podMap map[prometheus.PodKey]*prometheus.PodData, namespace string) *prometheus.PodData {
	container := "__unmounted__"
	podName := fmt.Sprintf("%s-unmounted-pvcs", namespace)

	thisPodKey := prometheus.PodKey{
		Namespace: namespace,
		Pod:       podName,
	}

	// Initialize pod and container if they do not already exist
	thisPod, ok := podMap[thisPodKey]
	if !ok {
		thisPod = &prometheus.PodData{
			Start:      window.Start,
			End:        window.End,
			Containers: make(map[string]map[string]*prometheus.PVAllocations),
		}

		thisPod.Containers[container] = make(map[string]*prometheus.PVAllocations)
		podMap[thisPodKey] = thisPod
	}
	return thisPod
}
