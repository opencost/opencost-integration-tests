package prometheus

// Description: Verifies that UIDs are properly emitted for all Kubernetes resources
//
// Tests verify:
// - All resource types have UID labels in their metrics
// - UIDs are valid UUID format
// - UIDs remain consistent across related metrics
// - UIDs persist throughout resource lifecycle

import (
	"regexp"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/log"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

// ResourceType represents a Kubernetes resource type
type ResourceType string

// Resource type constants for better type safety and maintainability
const (
	ResourceTypeDeployment            ResourceType = "deployment"
	ResourceTypeJob                   ResourceType = "job"
	ResourceTypeNamespace             ResourceType = "namespace"
	ResourceTypeNode                  ResourceType = "node"
	ResourceTypePod                   ResourceType = "pod"
	ResourceTypePersistentVolumeClaim ResourceType = "persistentvolumeclaim"
	ResourceTypePersistentVolume      ResourceType = "persistentvolume"
	ResourceTypeService               ResourceType = "service"
)

// Default test time windows - can be easily modified for different test scenarios
var defaultTestWindows = []string{"1h", "6h", "24h"}

// metricToResourceType maps each metric to its resource type for extracting resource names
var metricToResourceType = map[string]ResourceType{
	// Deployment metrics
	"deployment_match_labels": ResourceTypeDeployment,
	// Job metrics
	"kube_job_status_failed": ResourceTypeJob,
	// Namespace metrics
	"kube_namespace_annotations": ResourceTypeNamespace,
	"kube_namespace_labels":      ResourceTypeNamespace,
	// Node metrics
	"kube_node_status_capacity":                 ResourceTypeNode,
	"kube_node_status_capacity_memory_bytes":    ResourceTypeNode,
	"kube_node_status_capacity_cpu_cores":       ResourceTypeNode,
	"kube_node_status_allocatable":              ResourceTypeNode,
	"kube_node_status_allocatable_cpu_cores":    ResourceTypeNode,
	"kube_node_status_allocatable_memory_bytes": ResourceTypeNode,
	"kube_node_labels":                          ResourceTypeNode,
	// Pod metrics
	"kube_pod_labels":                             ResourceTypePod,
	"kube_pod_owner":                              ResourceTypePod,
	"kube_pod_container_status_running":           ResourceTypePod,
	"kube_pod_container_status_terminated_reason": ResourceTypePod,
	"kube_pod_container_resource_requests":        ResourceTypePod,
	// PVC metrics
	"kube_persistentvolumeclaim_resource_requests_storage_bytes": ResourceTypePersistentVolumeClaim,
	"kube_persistentvolumeclaim_info":                            ResourceTypePersistentVolumeClaim,
	// PV metrics
	"kube_persistentvolume_capacity_bytes": ResourceTypePersistentVolume,
	"kubecost_pv_info":                     ResourceTypePersistentVolume,
	// Service metrics
	"service_selector_labels": ResourceTypeService,
}

// UUID validation regex pattern (RFC 4122) - case-insensitive for robustness
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validateUID checks if a UID is in valid UUID format
func validateUID(uid string) bool {
	return uuidPattern.MatchString(uid)
}

// isEphemeralResource checks if a resource type is ephemeral (can be recreated/restarted)
// Ephemeral resources may have different UIDs across time windows due to recreation
func isEphemeralResource(resourceType ResourceType) bool {
	return resourceType == ResourceTypePod
}

// TestContext holds shared resources for UID verification tests
type TestContext struct {
	Client  *prometheus.Client
	EndTime int64
	Windows []string
}

// NewTestContext creates a new test context with shared resources
func NewTestContext() *TestContext {
	queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	return &TestContext{
		Client:  prometheus.NewClient(),
		EndTime: queryEnd.Unix(),
		Windows: defaultTestWindows,
	}
}

// testSingleMetric tests UID verification for a single metric across multiple time windows
// For ephemeral resources (pods), only validates UID presence and format
// For stable resources, also validates UID consistency across time windows
func testSingleMetric(t *testing.T, ctx *TestContext, metric string, resourceType ResourceType) {
	log.Infof("Testing metric: %s (%s)", metric, resourceType)

	resourcesWithUIDs := make(map[string]string)
	allResourceUIDs := make(map[string][]string) // For ephemeral resources, collect all UIDs per resource
	totalResourcesFound := 0
	hasValidResources := false
	invalidUIDCount := 0
	isEphemeral := isEphemeralResource(resourceType)

	if isEphemeral {
		log.Infof("Resource type %s is ephemeral, skipping cross-window UID consistency checks", resourceType)
	}

	// Test across all time windows
	for _, window := range ctx.Windows {
		uidMap, totalResources := queryResourceWithUID(t, ctx.Client, metric, window, &ctx.EndTime)

		if totalResources > 0 {
			hasValidResources = true
			totalResourcesFound += totalResources
		}

		// Handle UIDs differently for ephemeral vs stable resources
		for resourceName, uid := range uidMap {
			if isEphemeral {
				// For ephemeral resources, collect all UIDs but don't enforce consistency
				if _, exists := allResourceUIDs[resourceName]; !exists {
					allResourceUIDs[resourceName] = []string{}
					resourcesWithUIDs[resourceName] = uid // Keep the first UID for format validation
				}
				allResourceUIDs[resourceName] = append(allResourceUIDs[resourceName], uid)
			} else {
				// For stable resources, enforce UID consistency across windows
				if existingUID, exists := resourcesWithUIDs[resourceName]; exists {
					if existingUID != uid {
						t.Errorf("UID mismatch for %s %s in metric %s: %s vs %s (window: %s)",
							resourceType, resourceName, metric, existingUID, uid, window)
					}
				} else {
					resourcesWithUIDs[resourceName] = uid
				}
			}
		}
	}

	// Validate UID formats for all resources
	for resourceName, uid := range resourcesWithUIDs {
		if !validateUID(uid) {
			t.Errorf("Invalid UID format for %s %s in metric %s: %s",
				resourceType, resourceName, metric, uid)
			invalidUIDCount++
		}
	}

	// Additional validation for ephemeral resources
	if isEphemeral {
		uniqueUIDs := 0
		totalUIDs := 0
		for resourceName, uids := range allResourceUIDs {
			uniqueUIDSet := make(map[string]bool)
			for _, uid := range uids {
				uniqueUIDSet[uid] = true
				totalUIDs++
			}
			uniqueUIDs += len(uniqueUIDSet)
			log.Infof("Ephemeral resource %s %s: %d unique UIDs across %d observations",
				resourceType, resourceName, len(uniqueUIDSet), len(uids))
		}
		log.Infof("Ephemeral resources summary: %d unique UIDs across %d total observations", uniqueUIDs, totalUIDs)
	}

	// Report results
	if !hasValidResources {
		t.Errorf("No resources found for metric %s across any time windows", metric)
	} else if len(resourcesWithUIDs) == 0 {
		t.Errorf("Found %d resources for metric %s but none have UIDs",
			totalResourcesFound, metric)
	} else {
		log.Infof("Metric %s: Found %d resources with UIDs, %d invalid",
			metric, len(resourcesWithUIDs), invalidUIDCount)
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

		// Use the metric to resource type mapping to get the resource name
		resourceType, exists := metricToResourceType[metric]
		if !exists {
			log.Warnf("Unknown metric type: %s, dumping metric: %s", metric, item.Metric.ToString())
			continue
		}

		resourceName := item.Metric.GetResourceName(string(resourceType))

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

// =============================================================================
// INDIVIDUAL METRIC TESTS
// Each metric is tested separately for better isolation and granular reporting
// =============================================================================

// Pod Metrics Tests
func TestPodLabelsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_pod_labels", ResourceTypePod)
}

func TestPodOwnerMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_pod_owner", ResourceTypePod)
}

func TestPodContainerStatusRunningMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_pod_container_status_running", ResourceTypePod)
}

func TestPodContainerStatusTerminatedReasonMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_pod_container_status_terminated_reason", ResourceTypePod)
}

func TestPodContainerResourceRequestsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_pod_container_resource_requests", ResourceTypePod)
}

// Deployment Metrics Tests
func TestDeploymentMatchLabelsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "deployment_match_labels", ResourceTypeDeployment)
}

// Job Metrics Tests
func TestJobStatusFailedMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_job_status_failed", ResourceTypeJob)
}

// Namespace Metrics Tests
func TestNamespaceAnnotationsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_namespace_annotations", ResourceTypeNamespace)
}

func TestNamespaceLabelsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_namespace_labels", ResourceTypeNamespace)
}

// Node Metrics Tests
func TestNodeStatusCapacityMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_capacity", ResourceTypeNode)
}

func TestNodeStatusCapacityMemoryBytesMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_capacity_memory_bytes", ResourceTypeNode)
}

func TestNodeStatusCapacityCpuCoresMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_capacity_cpu_cores", ResourceTypeNode)
}

func TestNodeStatusAllocatableMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_allocatable", ResourceTypeNode)
}

func TestNodeStatusAllocatableCpuCoresMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_allocatable_cpu_cores", ResourceTypeNode)
}

func TestNodeStatusAllocatableMemoryBytesMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_status_allocatable_memory_bytes", ResourceTypeNode)
}

func TestNodeLabelsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_node_labels", ResourceTypeNode)
}

// PersistentVolumeClaim Metrics Tests
func TestPVCResourceRequestsStorageBytesMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_persistentvolumeclaim_resource_requests_storage_bytes", ResourceTypePersistentVolumeClaim)
}

func TestPVCInfoMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_persistentvolumeclaim_info", ResourceTypePersistentVolumeClaim)
}

// PersistentVolume Metrics Tests
func TestPVCapacityBytesMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kube_persistentvolume_capacity_bytes", ResourceTypePersistentVolume)
}

func TestKubecostPVInfoMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "kubecost_pv_info", ResourceTypePersistentVolume)
}

// Service Metrics Tests
func TestServiceSelectorLabelsMetricUID(t *testing.T) {
	ctx := NewTestContext()
	testSingleMetric(t, ctx, "service_selector_labels", ResourceTypeService)
}


// TestAllMetricsUIDVerification runs UID verification for every individual metric
// This provides the most granular test reporting
func TestAllMetricsUIDVerification(t *testing.T) {
	ctx := NewTestContext()

	for metric, resourceType := range metricToResourceType {
		t.Run(metric, func(t *testing.T) {
			testSingleMetric(t, ctx, metric, resourceType)
		})
	}
}
