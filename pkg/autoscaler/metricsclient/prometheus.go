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
	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

const (
	functionLabelName = "function"     // For nuclio functions (nuclio_processor_handled_events_total) - maps to nuclio function resource
	serviceLabelName  = "service_name" // For deployments (num_of_requests, jupyter_kernel_busyness) - maps to deployment resource
	podLabelName      = "pod"          // For pod-based metrics (DCGM_FI_DEV_GPU_UTIL) - maps to pod resource
)

// windowSizeLookup maps windowSize → set of resourceNames
type windowSizeLookup map[string]map[string]struct{}

// metricLookup maps metricName → windowSizeLookup
type metricLookup map[string]windowSizeLookup

type PrometheusMetricsClient struct {
	logger         logger.Logger
	apiClient      prometheusv1.API
	namespace      string
	queryTemplates map[string]*template.Template
}

func NewPrometheusClient(parentLogger logger.Logger, prometheusURL, namespace string, templates []scalertypes.QueryTemplate) (*PrometheusMetricsClient, error) {
	if len(templates) == 0 {
		return nil, errors.New("query template cannot be empty")
	}

	if prometheusURL == "" {
		return nil, errors.New("prometheus URL cannot be empty")
	}

	if namespace == "" {
		return nil, errors.New("namespace cannot be empty")
	}

	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: prometheusURL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create prometheus API client")
	}

	queryTemplates := make(map[string]*template.Template)
	for _, queryTemplate := range templates {
		tmpl, err := queryTemplate.CreateQueryTemplate()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create query template")
		}
		queryTemplates[queryTemplate.Name] = tmpl
	}

	childLogger := parentLogger.GetChild("prometheus-client")
	childLogger.Info("Creating prometheus metrics client")

	return &PrometheusMetricsClient{
		logger:         childLogger,
		apiClient:      prometheusv1.NewAPI(client),
		namespace:      namespace,
		queryTemplates: queryTemplates,
	}, nil
}

// GetResourceMetrics retrieves metrics for multiple resources
func (pc *PrometheusMetricsClient) GetResourceMetrics(resources []scalertypes.Resource) (map[string]map[string]int, error) {
	metricsByResource := make(map[string]map[string]int)
	metricToWindowSizes := pc.buildMetricLookup(resources)

	for metricName, queryTemplate := range pc.queryTemplates {
		windowSizeToResources := metricToWindowSizes[metricName]

		for windowSize, resourcesInWindowSize := range windowSizeToResources {
			// create resource name regex for Prometheus query based on the resources in this window size
			resourceNameRegex := pc.createResourceNameRegex(resourcesInWindowSize)
			fullMetricName := scalertypes.GetKubernetesMetricName(metricName, windowSize)
			query, err := pc.renderQuery(queryTemplate, windowSize, resourceNameRegex)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to render query for metricName=%s, windowSize=%s", metricName, windowSize)
			}

			rawResult, warnings, err := pc.apiClient.Query(context.Background(), query, time.Now())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to execute Prometheus query for metricName=%s, windowSize=%s", metricName, windowSize)
			}

			if len(warnings) > 0 {
				pc.logger.WarnWith("Prometheus query returned warnings",
					"metricName", metricName,
					"windowSize", windowSize,
					"warnings", warnings)
			}

			metricSamples, ok := rawResult.(model.Vector)
			if !ok {
				return nil, errors.Wrapf(err, "unexpected Prometheus result type for metricName=%s, windowSize=%s", metricName, windowSize)
			}

			for _, metricSample := range metricSamples {
				resourceName, err := pc.extractResourceName(metricSample.Metric)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to extract resource name from the prometheus metric's labels. metricName=%s, windowSize=%s", metricName, windowSize)
				}

				if _, exists := resourcesInWindowSize[resourceName]; !exists {
					pc.logger.DebugWith("Received metric for unconfigured resource, skipping",
						"resourceName", resourceName,
						"metricName", metricName,
						"windowSize", windowSize)
					continue
				}

				// Round up values to ensure any fractional value > 0 becomes at least 1
				// This prevents incorrect scale-to-zero decisions for resources with low activity
				metricValue := int(math.Ceil(float64(metricSample.Value)))

				if _, exists := metricsByResource[resourceName]; !exists {
					metricsByResource[resourceName] = make(map[string]int)
				}

				pc.logger.DebugWith("Retrieved metric",
					"resourceName", resourceName,
					"metricName", fullMetricName,
					"windowSize", windowSize,
					"value", metricValue)

				if _, exists := metricsByResource[resourceName][fullMetricName]; exists {
					return nil, errors.Errorf("Cannot have more than one metricSample value per resource: resource=%s, metricSample=%s", resourceName, fullMetricName)
				}
				metricsByResource[resourceName][fullMetricName] = metricValue
			}
		}
	}
	return metricsByResource, nil
}

// renderQuery renders the Prometheus query template
func (pc *PrometheusMetricsClient) renderQuery(queryTemplate *template.Template, windowSize, resourceNameRegex string) (string, error) {
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

// buildMetricLookup builds a nested lookup structure from resources
func (pc *PrometheusMetricsClient) buildMetricLookup(resources []scalertypes.Resource) metricLookup {
	lookup := make(metricLookup)

	for _, resource := range resources {
		for _, scaleResource := range resource.ScaleResources {
			metricName := scaleResource.MetricName
			windowSize := scalertypes.ShortDurationString(scaleResource.WindowSize)

			if lookup[metricName] == nil {
				lookup[metricName] = make(windowSizeLookup)
			}
			if lookup[metricName][windowSize] == nil {
				lookup[metricName][windowSize] = make(map[string]struct{})
			}

			lookup[metricName][windowSize][resource.Name] = struct{}{}
		}
	}

	return lookup
}

// extractResourceName extracts the resource name from Prometheus metric labels.
func (pc *PrometheusMetricsClient) extractResourceName(labels model.Metric) (string, error) {
	labelNames := []model.LabelName{
		functionLabelName,
		serviceLabelName,
		podLabelName,
	}
	for _, labelName := range labelNames {
		if value, ok := labels[labelName]; ok {
			return string(value), nil
		}
	}
	return "", errors.Errorf("Could not extract resource name from labels: %v", labels)
}

// createResourceNameRegex creates a regex string for Prometheus query from resource names
func (pc *PrometheusMetricsClient) createResourceNameRegex(resources map[string]struct{}) string {
	resourcesNames := make([]string, len(resources))
	i := 0
	for resourceName := range resources {
		resourcesNames[i] = resourceName
		i++
	}
	return strings.Join(resourcesNames, "|")
}
