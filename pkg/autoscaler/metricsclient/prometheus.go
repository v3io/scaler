/*
Copyright 2026 Iguazio Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License") with
an addition restriction as set forth herein. You may not use this
file except in compliance with the License. You may obtain a copy of
the License at http://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing
permissions and limitations under the License.

In addition, you may not use the software for any purposes that are
illegal under applicable law, and the grant of the foregoing license
under the Apache 2.0 license is conditioned upon your compliance with
such restriction.
*/

package metricsclient

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"text/template"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type PrometheusClient struct {
	logger         logger.Logger
	apiClient      v1.API
	namespace      string
	queryTemplates map[string]*template.Template
}

func NewPrometheusClient(parentLogger logger.Logger, prometheusURL, namespace string, templates []scalertypes.QueryTemplate) (*PrometheusClient, error) {
	if len(templates) == 0 {
		return nil, errors.New("Failed to created Prometheus client: query template cannot be empty")
	}

	if prometheusURL == "" {
		return nil, errors.New("Failed to created Prometheus client: prometheus URL cannot be empty")
	}

	if namespace == "" {
		return nil, errors.New("Failed to created Prometheus client: namespace cannot be empty")
	}

	client, err := api.NewClient(api.Config{
		Address: prometheusURL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to created Prometheus client: failed to create prometheus API client")
	}

	queryTemplates := make(map[string]*template.Template)
	for _, queryTemplate := range templates {
		if queryTemplate.Name == "" {
			return nil, errors.New("Failed to created Prometheus client: template name cannot be empty")
		}
		if queryTemplate.Template == "" {
			return nil, errors.New("Failed to created Prometheus client: query template cannot be empty")
		}
		tmpl, err := template.New(queryTemplate.Name).Parse(queryTemplate.Template)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to created Prometheus client: failed to parse template in prometheus client")
		}
		queryTemplates[queryTemplate.Name] = tmpl
	}

	childLogger := parentLogger.GetChild("prometheus-client")
	childLogger.Info("Creating prometheus client")

	return &PrometheusClient{
		logger:         childLogger,
		apiClient:      v1.NewAPI(client),
		namespace:      namespace,
		queryTemplates: queryTemplates,
	}, nil
}

// renderQuery renders the Prometheus query template
func (pc *PrometheusClient) renderQuery(queryTemplate *template.Template, windowSize, resourceNameRegex string) (string, error) {
	templateData := make(map[string]string)
	templateData["Namespace"] = pc.namespace
	templateData["WindowSize"] = windowSize
	templateData["Resources"] = resourceNameRegex

	var queryBuffer bytes.Buffer
	if err := queryTemplate.Execute(&queryBuffer, templateData); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return queryBuffer.String(), nil
}

// extractWindowSizesForMetric extracts unique window sizes from resources' ScaleResources for a specific metric name.
func (pc *PrometheusClient) extractWindowSizesForMetric(resources []scalertypes.Resource, metricName string) map[string]bool {
	windowSizes := make(map[string]bool)
	for _, resource := range resources {
		for _, scaleResource := range resource.ScaleResources {
			if scaleResource.MetricName == metricName {
				windowSizeStr := scalertypes.ShortDurationString(scaleResource.WindowSize)
				windowSizes[windowSizeStr] = true
			}
		}
	}
	return windowSizes
}

// buildResourceNameRegex creates a pipe-separated regex pattern from resource names for Prometheus query filtering (e.g., "func1|func2|func3")
func (pc *PrometheusClient) buildResourceNameRegex(resources []scalertypes.Resource) string {
	resourceNames := make([]string, len(resources))
	for i, resource := range resources {
		resourceNames[i] = resource.Name
	}
	return strings.Join(resourceNames, "|")
}

// resolveFullMetricName finds the matching ScaleResource for a given metric name and window size,
// and returns the full metric name (e.g., "metric_name_per_1m").
func (pc *PrometheusClient) resolveFullMetricName(resources []scalertypes.Resource, metricName, windowSize string) (string, error) {
	for _, resource := range resources {
		for _, scaleResource := range resource.ScaleResources {
			if scaleResource.MetricName == metricName {
				scaleResourceWindowSize := scalertypes.ShortDurationString(scaleResource.WindowSize)
				if scaleResourceWindowSize == windowSize {
					return scaleResource.GetKubernetesMetricName(), nil
				}
			}
		}
	}
	return "", errors.Errorf("Failed to find ScaleResource matching metric name and window size: metricName=%s, windowSize=%s", metricName, windowSize)
}

// GetResourceMetrics retrieves metrics for multiple resources
func (pc *PrometheusClient) GetResourceMetrics(resources []scalertypes.Resource) (map[string]map[string]int, error) {
	metricsByResource := make(map[string]map[string]int)

	for metricName, queryTemplate := range pc.queryTemplates {
		windowSizes := pc.extractWindowSizesForMetric(resources, metricName)
		if len(windowSizes) == 0 {
			pc.logger.DebugWith("No window sizes found for metric, skipping",
				"metricName", metricName)
			continue
		}

		resourceNameRegex := pc.buildResourceNameRegex(resources)

		for windowSize := range windowSizes {
			fullMetricName, err := pc.resolveFullMetricName(resources, metricName, windowSize)
			if err != nil {
				pc.logger.WarnWith("Could not find matching ScaleResource for metricName and window size, skipping",
					"metricName", metricName,
					"windowSize", windowSize,
					"error", err)
				continue
			}

			query, err := pc.renderQuery(queryTemplate, windowSize, resourceNameRegex)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed to render query for metricName=%s, windowSize=%s", metricName, windowSize)
			}

			rawResult, warnings, err := pc.apiClient.Query(context.Background(), query, time.Now())
			if err != nil {
				pc.logger.WarnWith("Failed to execute Prometheus query",
					"metricName", metricName,
					"windowSize", windowSize,
					"error", err)
				continue
			}

			if len(warnings) > 0 {
				pc.logger.DebugWith("Prometheus query warnings",
					"metricName", metricName,
					"windowSize", windowSize,
					"warnings", warnings)
			}

			metricSamples, ok := rawResult.(model.Vector)
			if !ok {
				pc.logger.WarnWith("Unexpected Prometheus result type",
					"metricName", metricName,
					"windowSize", windowSize,
					"expectedType", "model.Vector",
					"actualType", fmt.Sprintf("%T", rawResult))
				continue
			}

			for _, metricSample := range metricSamples {
				resourceName, err := pc.extractResourceName(metricSample.Metric)
				if err != nil {
					pc.logger.WarnWith("Failed to extract resource name from Prometheus metricSample labels",
						"metricName", metricName,
						"windowSize", windowSize,
						"labels", metricSample.Metric.String(),
						"error", err)
					continue
				}

				// Use Ceil to ensure any fractional value > 0 becomes at least 1
				// This prevents incorrect scale-to-zero decisions for resources with low activity
				metricValue := int(math.Ceil(float64(metricSample.Value)))

				if _, found := metricsByResource[resourceName]; !found {
					metricsByResource[resourceName] = make(map[string]int)
				}

				pc.logger.DebugWith("Retrieved metricSample sample",
					"resourceName", resourceName,
					"metricName", fullMetricName,
					"windowSize", windowSize,
					"value", metricValue)

				if _, found := metricsByResource[resourceName][fullMetricName]; found {
					return nil, errors.Errorf("Cannot have more than one metricSample value per resource: resource=%s, metricSample=%s", resourceName, fullMetricName)
				}

				metricsByResource[resourceName][fullMetricName] = metricValue
			}
		}
	}

	return metricsByResource, nil
}

// extractResourceName extracts the resource name from Prometheus metric labels.
func (pc *PrometheusClient) extractResourceName(labels model.Metric) (string, error) {
	labelNames := []model.LabelName{
		"function",     // For nuclio functions (nuclio_processor_handled_events_total) - maps to nucliofunction resource
		"service_name", // For deployments (num_of_requests, jupyter_kernel_busyness) - maps to deployment resource
		"pod",          // For pod-based metrics (DCGM_FI_DEV_GPU_UTIL) - maps to pod resource
	}

	for _, labelName := range labelNames {
		if value, ok := labels[labelName]; ok {
			return string(value), nil
		}
	}

	// If no common label found, return error
	return "", errors.Errorf("Could not extract resource name from labels: %v", labels)
}
