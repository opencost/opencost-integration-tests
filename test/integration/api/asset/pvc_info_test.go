package assets

// ### Description
// Compare PVC Disks from Assets API and from Promethues

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const Resolution = "5m"

func getOffsetAdjustedQueryWindow(window string) string {

	// This function is specifically designed for window is [0-9]h format and resolution in [0-9]m.
	// Please upgrade this function if you want to support more time ranges or special keywords.
	window_int, _ := utils.ExtractNumericPrefix(window)
	resolution_int, _ := utils.ExtractNumericPrefix(Resolution)

	window_offset := strconv.Itoa(int(window_int)*60 + int(resolution_int))
	window_offset_string := fmt.Sprintf("%sm", window_offset)

	return window_offset_string
}

func queryPVCInfo(window string, endTime int64, t *testing.T) (prometheus.PrometheusResponse, error) {

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

	pvcInfo, err := client.RunPromQLQuery(promPVCInfo, t)

	return pvcInfo, err
}

func TestPVCInfo(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "Today",
			window:    "24h",
			assetType: "disk",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			pvcInfo, err := queryPVCInfo(tc.window, endTime, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			if len(pvcInfo.Data.Result) == 0 {
				t.Fatalf("No Disks Found. Failing Test")
			}
			// Store Results in a Node Map
			type DiskPVCInfo struct {
				Volume         string
				ClaimNameProm  string
				ClaimNameAsset string
				IsVolumeInBoth bool
			}

			diskPVCInfoMap := make(map[string]*DiskPVCInfo)

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

				diskPVCInfoMap[persistentVolumeName] = &DiskPVCInfo{
					Volume:         persistentVolumeName,
					ClaimNameProm:  persistentVolumeClaimName,
					IsVolumeInBoth: false,
				}
			}

			// API Response
			apiResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetType,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Store Allocation Pod Label Results
			for _, assetResponseItem := range apiResponse.Data {
				volume := assetResponseItem.VolumeName
				claimName := assetResponseItem.ClaimName

				disk, ok := diskPVCInfoMap[volume]
				if !ok {
					t.Logf("Node Information Missing from Prometheus %s", volume)
					continue
				}

				if (volume == disk.Volume) && (disk.ClaimNameProm == claimName) {
					disk.IsVolumeInBoth = true
				}
				disk.ClaimNameAsset = claimName
			}

			// Compare Results
			for volume, diskPVCValues := range diskPVCInfoMap {
				t.Logf("Volume: %s", volume)

				if diskPVCValues.IsVolumeInBoth == true {
					t.Logf("  - [Pass]: ClaimName: %s", diskPVCValues.ClaimNameAsset)
				} else {
					t.Errorf("  - [Fail]: Claim Alloc %s != Claim Prom %s", diskPVCValues.ClaimNameAsset, diskPVCValues.ClaimNameProm)
				}
			}
		})
	}
}
