package prometheus

// Description: Verifies that UIDs are properly emitted for all Kubernetes resources
// as per OpenCost PR #3366 (https://github.com/opencost/opencost/pull/3366)
//
// Tests verify:
// - All resource types have UID labels in their metrics
// - UIDs are valid UUID format
// - UIDs remain consistent across related metrics
// - UIDs persist throughout resource lifecycle

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/log"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

// UUID validation regex pattern (RFC 4122) - case-insensitive for robustness
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validateUID checks if a UID is in valid UUID format
func validateUID(uid string) bool {
	return uuidPattern.MatchString(uid)
}

// testResourceUIDVerification is a generic helper function to test UID verification for any resource type
func testResourceUIDVerification(t *testing.T, resourceType string, metrics []string) {
	client := prometheus.NewClient()
	queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	endTime := queryEnd.Unix()
	window := "24h"

	log.Infof("Testing %s UID verification", resourceType)

	allUIDs := make(map[string]string)
	seenResources := make(map[string]bool)
	totalUniqueResources := 0

	// Loop through all metrics for this resource type
	for _, metric := range metrics {
		log.Infof("Checking metric: %s", metric)
		uidMap, _ := queryResourceWithUID(t, client, metric, window, &endTime)

		// Process each resource found in this metric
		for resourceName, uid := range uidMap {
			// Track unique resources to avoid double counting
			if !seenResources[resourceName] {
				totalUniqueResources++
				seenResources[resourceName] = true
			}

			// Check for UID consistency across metrics
			if existingUID, exists := allUIDs[resourceName]; exists {
				if existingUID != uid {
					t.Errorf("UID mismatch for %s %s: %s vs %s (metric: %s)", resourceType, resourceName, existingUID, uid, metric)
				}
			} else {
				allUIDs[resourceName] = uid
			}
		}
	}

	// Skip if no resources exist at all
	if totalUniqueResources == 0 {
		log.Warnf("No %s found in metrics", resourceType)
		t.Skipf("Skipping: No %s found in cluster", resourceType)
		return
	}

	// Fail if resources exist but no UIDs
	if len(allUIDs) == 0 {
		t.Errorf("Found %d %s but none have UIDs - OpenCost UID support not properly deployed", totalUniqueResources, resourceType)
		return
	}

	// Validate UID formats
	invalidUIDs := 0
	for resourceName, uid := range allUIDs {
		if !validateUID(uid) {
			t.Errorf("%s %s has invalid UID format: %s", resourceType, resourceName, uid)
			invalidUIDs++
		}
	}

	log.Infof("Found %d %s with UIDs, %d invalid", len(allUIDs), resourceType, invalidUIDs)

	if invalidUIDs > 0 {
		t.Errorf("Found %d %s with invalid UID format", invalidUIDs, resourceType)
	}
}

// queryResourceWithUID queries Prometheus for a metric and returns both total resources and those with UIDs
func queryResourceWithUID(t *testing.T, client *prometheus.Client, metric string, window string, endTime *int64) (uidMap map[string]string, totalResources int) {
	input := prometheus.PrometheusInput{
		Metric:      metric,
		Function:    []string{"avg_over_time"},
		QueryWindow: window,
		Time:        endTime,
	}

	result, err := client.RunPromQLQuery(input)
	if err != nil {
		t.Fatalf("Error querying Prometheus for %s: %v", metric, err)
	}

	uidMap = make(map[string]string)
	totalResources = 0

	for _, item := range result.Data.Result {
		// Extract UID from labels
		uid := item.Metric.UID
		resourceName := ""

		// Different metrics have different label names and resource name formats
		// Resource naming: namespace-scoped resources use "namespace/name" format,
		// cluster-scoped resources (nodes, PVs, namespaces) use just the name
		switch metric {
		// Deployment metrics
		case "deployment_match_labels", "kube_deployment_spec_replicas", 
			 "kube_deployment_status_replicas_available":
			if item.Metric.Deployment != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.Deployment) // namespace-scoped
			}
		// Job metrics
		case "kube_job_status_failed":
			if item.Metric.JobName != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.JobName) // namespace-scoped
			}
		// Namespace metrics
		case "kube_namespace_annotations", "kube_namespace_labels":
			if item.Metric.Namespace != "" {
				resourceName = item.Metric.Namespace // cluster-scoped
			}
		// Node metrics
		case "kube_node_status_capacity", "kube_node_status_capacity_memory_bytes", 
			 "kube_node_status_capacity_cpu_cores", "kube_node_status_allocatable",
			 "kube_node_status_allocatable_cpu_cores", "kube_node_status_allocatable_memory_bytes",
			 "kube_node_labels", "kube_node_status_condition":
			if item.Metric.Node != "" {
				resourceName = item.Metric.Node // cluster-scoped
			}
		// Pod metrics
		case "kube_pod_labels":
			if item.Metric.Pod != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.Pod) // namespace-scoped
			}
		// PVC metrics
		case "kube_persistentvolumeclaim_resource_requests_storage_bytes", 
			 "kube_persistentvolumeclaim_info":
			if item.Metric.PersistentVolumeClaim != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.PersistentVolumeClaim) // namespace-scoped
			}
		// PV metrics
		case "kube_persistentvolume_capacity_bytes", "kube_persistentvolume_status_phase", 
			 "kubecost_pv_info":
			if item.Metric.PersistentVolume != "" {
				resourceName = item.Metric.PersistentVolume // cluster-scoped
			}
		// Service metrics
		case "service_selector_labels":
			if item.Metric.Service != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.Service) // namespace-scoped
			}
		// StatefulSet metrics
		case "statefulSet_match_labels":
			if item.Metric.StatefulSet != "" && item.Metric.Namespace != "" {
				resourceName = fmt.Sprintf("%s/%s", item.Metric.Namespace, item.Metric.StatefulSet) // namespace-scoped
			}
		}

		// Count all resources found
		if resourceName != "" {
			totalResources++
			// Only add to uidMap if UID is present
			if uid != "" {
				uidMap[resourceName] = uid
			}
		}
	}

	return uidMap, totalResources
}

// TestPodUIDVerification tests that pods have valid UIDs in its metrics
func TestPodUIDVerification(t *testing.T) {
	podMetrics := []string{"kube_pod_labels"}
	testResourceUIDVerification(t, "pods", podMetrics)
}

// TestDeploymentUIDVerification tests that deployments have valid UIDs in its metrics
func TestDeploymentUIDVerification(t *testing.T) {
	deploymentMetrics := []string{"deployment_match_labels", "kube_deployment_spec_replicas", 
		"kube_deployment_status_replicas_available"}
	testResourceUIDVerification(t, "deployments", deploymentMetrics)
}

// TestStatefulSetUIDVerification tests that statefulsets have valid UIDs in its metrics
func TestStatefulSetUIDVerification(t *testing.T) {
	statefulSetMetrics := []string{"statefulSet_match_labels"}
	testResourceUIDVerification(t, "statefulsets", statefulSetMetrics)
}

// TestServiceUIDVerification tests that services have valid UIDs in its metrics
func TestServiceUIDVerification(t *testing.T) {
	serviceMetrics := []string{"service_selector_labels"}
	testResourceUIDVerification(t, "services", serviceMetrics)
}

// TestNamespaceUIDVerification tests that namespaces have valid UIDs in its metrics
func TestNamespaceUIDVerification(t *testing.T) {
	namespaceMetrics := []string{"kube_namespace_annotations", "kube_namespace_labels"}
	testResourceUIDVerification(t, "namespaces", namespaceMetrics)
}

// TestNodeUIDVerification tests that nodes have valid UIDs in its metrics
func TestNodeUIDVerification(t *testing.T) {
	nodeMetrics := []string{
		"kube_node_status_capacity", "kube_node_status_capacity_memory_bytes", 
		"kube_node_status_capacity_cpu_cores", "kube_node_status_allocatable",
		"kube_node_status_allocatable_cpu_cores", "kube_node_status_allocatable_memory_bytes",
		"kube_node_labels", "kube_node_status_condition",
	}
	testResourceUIDVerification(t, "nodes", nodeMetrics)
}

// TestPersistentVolumeUIDVerification tests that PVs have valid UIDs in its metrics
func TestPersistentVolumeUIDVerification(t *testing.T) {
	pvMetrics := []string{"kube_persistentvolume_capacity_bytes", 
		"kube_persistentvolume_status_phase", "kubecost_pv_info"}
	testResourceUIDVerification(t, "persistent volumes", pvMetrics)
}

// TestPersistentVolumeClaimUIDVerification tests that PVCs have valid UIDs in its metrics
func TestPersistentVolumeClaimUIDVerification(t *testing.T) {
	pvcMetrics := []string{"kube_persistentvolumeclaim_resource_requests_storage_bytes", 
		"kube_persistentvolumeclaim_info"}
	testResourceUIDVerification(t, "persistent volume claims", pvcMetrics)
}

// TestJobUIDVerification tests that jobs have valid UIDs in its metrics
func TestJobUIDVerification(t *testing.T) {
	jobMetrics := []string{"kube_job_status_failed"}
	testResourceUIDVerification(t, "jobs", jobMetrics)
}
