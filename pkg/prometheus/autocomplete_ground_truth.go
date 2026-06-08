package prometheus

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const (
	runningPodMetric = "kube_pod_container_status_running"
	podOwnerMetric   = "kube_pod_owner"
	podLabelsMetric  = "kube_pod_labels"
	nsLabelsMetric   = "kube_namespace_labels"
	podInfoMetric    = "kube_pod_info"
	nodeInfoMetric   = "kube_node_info"
	nodeCostMetric   = "node_total_hourly_cost"
	clusterInfoMetric = "kubecost_cluster_info"
)

// RunningPodKey identifies a running container workload in Prometheus.
type RunningPodKey struct {
	Namespace string
	Pod       string
	Container string
}

// RunningPods returns container/pod/namespace series with average running status > 0.
func (c *Client) RunningPods(window string, endTime int64) ([]RunningPodKey, error) {
	input := PrometheusInput{
		Metric:      runningPodMetric,
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: window,
		AggregateBy: []string{"namespace", "pod", "container"},
		Time:        &endTime,
	}

	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}

	var pods []RunningPodKey
	for _, result := range resp.Data.Result {
		if result.Value.Value <= 0 {
			continue
		}
		pods = append(pods, RunningPodKey{
			Namespace: result.Metric.Namespace,
			Pod:       result.Metric.Pod,
			Container: result.Metric.Container,
		})
	}
	return pods, nil
}

// AllocationFieldValues returns distinct values for an allocation autocomplete field.
func (c *Client) AllocationFieldValues(field, window string, endTime int64) (map[string]struct{}, error) {
	switch strings.ToLower(field) {
	case "namespace":
		return c.distinctFromRunningPods(window, endTime, func(p RunningPodKey) string { return p.Namespace })
	case "pod":
		return c.distinctFromRunningPods(window, endTime, func(p RunningPodKey) string { return p.Pod })
	case "container":
		return c.distinctFromRunningPods(window, endTime, func(p RunningPodKey) string { return p.Container })
	case "node":
		return c.allocationNodes(window, endTime)
	case "cluster":
		return c.allocationClusters(endTime)
	case "controllerkind":
		values, err := c.podOwnerField(window, endTime, "owner_kind")
		if err != nil {
			return nil, err
		}
		// OpenCost surfaces workload controllers (Deployment, etc.), not intermediate owners.
		for _, skip := range []string{"replicaset", "pod", "job", "cronjob"} {
			delete(values, skip)
		}
		return values, nil
	case "controllername":
		return c.podOwnerField(window, endTime, "owner_name")
	case "label":
		return c.podLabelKeys(window, endTime)
	default:
		if strings.HasPrefix(strings.ToLower(field), "label:") {
			labelKey := strings.TrimPrefix(field, "label:")
			labelKey = strings.TrimPrefix(labelKey, "Label:")
			return c.podLabelValues(window, endTime, labelKey)
		}
		if field == "namespacelabel" {
			return c.namespaceLabelKeys(window, endTime)
		}
		if strings.HasPrefix(strings.ToLower(field), "namespacelabel:") {
			labelKey := strings.TrimPrefix(field, "namespacelabel:")
			labelKey = strings.TrimPrefix(labelKey, "NamespaceLabel:")
			return c.namespaceLabelValues(window, endTime, labelKey)
		}
		return nil, fmt.Errorf("unsupported allocation autocomplete field for Prometheus ground truth: %s", field)
	}
}

// AssetFieldValues returns distinct values for an assets autocomplete field (node assets).
func (c *Client) AssetFieldValues(field, window string, endTime int64) (map[string]struct{}, error) {
	switch strings.ToLower(field) {
	case "name":
		return c.nodeNames(window, endTime)
	case "cluster":
		return c.allocationClusters(endTime)
	case "label":
		return c.nodeLabelKeys(window, endTime)
	case "provider":
		return c.nodeInfoField(window, endTime, "provider_id")
	case "providerid":
		return c.nodeInfoField(window, endTime, "provider_id")
	case "type":
		// OpenCost autocomplete returns canonical asset types in lowercase.
		return map[string]struct{}{"node": {}}, nil
	case "category":
		return map[string]struct{}{"Compute": {}}, nil
	default:
		if strings.HasPrefix(strings.ToLower(field), "label:") {
			labelKey := strings.TrimPrefix(field, "label:")
			return c.nodeLabelValues(window, endTime, labelKey)
		}
		return nil, fmt.Errorf("unsupported asset autocomplete field for Prometheus ground truth: %s", field)
	}
}

func (c *Client) distinctFromRunningPods(window string, endTime int64, extract func(RunningPodKey) string) (map[string]struct{}, error) {
	pods, err := c.RunningPods(window, endTime)
	if err != nil {
		return nil, err
	}
	values := make(map[string]struct{})
	for _, pod := range pods {
		v := strings.TrimSpace(extract(pod))
		if v != "" {
			values[v] = struct{}{}
		}
	}
	return values, nil
}

func (c *Client) allocationNodes(window string, endTime int64) (map[string]struct{}, error) {
	running, err := c.RunningPods(window, endTime)
	if err != nil {
		return nil, err
	}
	runningSet := make(map[string]struct{}, len(running))
	for _, p := range running {
		runningSet[podKey(p.Namespace, p.Pod)] = struct{}{}
	}

	input := PrometheusInput{
		Metric:      podInfoMetric,
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: window,
		AggregateBy: []string{"namespace", "pod", "node"},
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		if result.Value.Value <= 0 {
			continue
		}
		if _, ok := runningSet[podKey(result.Metric.Namespace, result.Metric.Pod)]; !ok {
			continue
		}
		if result.Metric.Node != "" {
			nodes[result.Metric.Node] = struct{}{}
		}
	}
	return nodes, nil
}

func (c *Client) allocationClusters(endTime int64) (map[string]struct{}, error) {
	// kubecost_cluster_info is an instant gauge; query at "now" because future-aligned
	// integration test timestamps may not have samples.
	now := time.Now().UTC().Unix()
	input := PrometheusInput{
		Metric: clusterInfoMetric,
		Time:   &now,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}
	clusters := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		id := result.Metric.UnhandledFields["id"]
		if id == "" {
			id = result.Metric.UnhandledFields["name"]
		}
		if id != "" {
			clusters[id] = struct{}{}
		}
	}
	if len(clusters) == 0 {
		return map[string]struct{}{"default-cluster": {}}, nil
	}
	return clusters, nil
}

func (c *Client) podOwnerField(window string, endTime int64, ownerField string) (map[string]struct{}, error) {
	running, err := c.RunningPods(window, endTime)
	if err != nil {
		return nil, err
	}
	runningSet := make(map[string]struct{}, len(running))
	for _, p := range running {
		runningSet[podKey(p.Namespace, p.Pod)] = struct{}{}
	}

	aggregateBy := []string{"namespace", "pod", ownerField}
	input := PrometheusInput{
		Metric:      podOwnerMetric,
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: window,
		AggregateBy: aggregateBy,
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}

	values := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		if result.Value.Value <= 0 {
			continue
		}
		if _, ok := runningSet[podKey(result.Metric.Namespace, result.Metric.Pod)]; !ok {
			continue
		}
		var v string
		switch ownerField {
		case "owner_kind":
			v = result.Metric.UnhandledFields["owner_kind"]
		case "owner_name":
			v = result.Metric.UnhandledFields["owner_name"]
		}
		if v != "" {
			values[strings.ToLower(v)] = struct{}{}
		}
	}
	return values, nil
}

func (c *Client) podLabelKeys(window string, endTime int64) (map[string]struct{}, error) {
	return c.labelKeysForMetric(window, endTime, podLabelsMetric, true)
}

func (c *Client) podLabelValues(window string, endTime int64, labelKey string) (map[string]struct{}, error) {
	return c.labelValuesForMetric(window, endTime, podLabelsMetric, labelKey, true)
}

func (c *Client) namespaceLabelKeys(window string, endTime int64) (map[string]struct{}, error) {
	return c.labelKeysForMetric(window, endTime, nsLabelsMetric, false)
}

func (c *Client) namespaceLabelValues(window string, endTime int64, labelKey string) (map[string]struct{}, error) {
	return c.labelValuesForMetric(window, endTime, nsLabelsMetric, labelKey, false)
}

func (c *Client) labelKeysForMetric(window string, endTime int64, metric string, filterRunning bool) (map[string]struct{}, error) {
	runningSet, err := c.runningPodSet(window, endTime, filterRunning)
	if err != nil {
		return nil, err
	}

	input := PrometheusInput{
		Metric:      metric,
		Function:    []string{"avg_over_time"},
		QueryWindow: window,
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		if filterRunning {
			if _, ok := runningSet[podKey(result.Metric.Namespace, result.Metric.Pod)]; !ok {
				continue
			}
		}
		for key := range result.Metric.Labels {
			keys[key] = struct{}{}
		}
	}
	return keys, nil
}

func (c *Client) labelValuesForMetric(window string, endTime int64, metric, labelKey string, filterRunning bool) (map[string]struct{}, error) {
	runningSet, err := c.runningPodSet(window, endTime, filterRunning)
	if err != nil {
		return nil, err
	}

	input := PrometheusInput{
		Metric:      metric,
		Function:    []string{"avg_over_time"},
		QueryWindow: window,
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}

	values := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		if filterRunning {
			if _, ok := runningSet[podKey(result.Metric.Namespace, result.Metric.Pod)]; !ok {
				continue
			}
		}
		if v, ok := result.Metric.Labels[labelKey]; ok && v != "" {
			values[v] = struct{}{}
		}
	}
	return values, nil
}

func (c *Client) nodeNames(window string, endTime int64) (map[string]struct{}, error) {
	input := PrometheusInput{
		Metric:      nodeCostMetric,
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: window,
		AggregateBy: []string{"node"},
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		if result.Metric.Node != "" && result.Value.Value > 0 {
			nodes[result.Metric.Node] = struct{}{}
		}
	}
	return nodes, nil
}

func (c *Client) nodeLabelKeys(window string, endTime int64) (map[string]struct{}, error) {
	return c.labelKeysForMetric(window, endTime, "kube_node_labels", false)
}

func (c *Client) nodeLabelValues(window string, endTime int64, labelKey string) (map[string]struct{}, error) {
	return c.labelValuesForMetric(window, endTime, "kube_node_labels", labelKey, false)
}

func (c *Client) nodeInfoField(window string, endTime int64, field string) (map[string]struct{}, error) {
	input := PrometheusInput{
		Metric:      nodeInfoMetric,
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: window,
		AggregateBy: []string{"node", field},
		Time:        &endTime,
	}
	resp, err := c.runPromQLQuery(input)
	if err != nil {
		return nil, err
	}
	values := make(map[string]struct{})
	for _, result := range resp.Data.Result {
		v := result.Metric.UnhandledFields[field]
		if v == "" {
			v = result.Metric.ProviderID
		}
		if v != "" && result.Value.Value > 0 {
			values[v] = struct{}{}
		}
	}
	return values, nil
}

// RunningPodsInWindowInput builds the pod-running PromQL OpenCost uses in QueryPods:
// avg(kube_pod_container_status_running != 0) by (...) [window:resolution]
// A pod is included if it was running during any resolution bucket in the window.
// Resolution defaults to 1m for demo.infra.opencost.io (queryResolutionSeconds: 60).
func RunningPodsInWindowInput(window string, endTime int64) PrometheusInput {
	resolutionMinutes := env.GetDataResolutionMinutes()
	return PrometheusInput{
		Metric:              runningPodMetric,
		MetricNotEqualTo:    "0",
		Function:            []string{"avg"},
		AggregateBy:         []string{"container", "pod", "namespace"},
		AggregateWindow:     window,
		AggregateResolution: fmt.Sprintf("%dm", resolutionMinutes),
		Time:                &endTime,
	}
}

func (c *Client) runningPodSet(window string, endTime int64, filterRunning bool) (map[string]struct{}, error) {
	if !filterRunning {
		return map[string]struct{}{}, nil
	}
	pods, err := c.RunningPods(window, endTime)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(pods))
	for _, p := range pods {
		set[podKey(p.Namespace, p.Pod)] = struct{}{}
	}
	return set, nil
}

func (c *Client) runPromQLQuery(input PrometheusInput) (PrometheusResponse, error) {
	promURL := c.ConstructPromQLQueryURL(input)
	promResp, err := c.httpClient.Get(promURL)
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer promResp.Body.Close()

	var promData PrometheusResponse
	if err := json.NewDecoder(promResp.Body).Decode(&promData); err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to decode Prometheus response: %w", err)
	}
	if promData.Status != "success" {
		return PrometheusResponse{}, fmt.Errorf("prometheus query unsuccessful: %s", promData.Status)
	}
	return promData, nil
}

func podKey(namespace, pod string) string {
	return namespace + "/" + pod
}
