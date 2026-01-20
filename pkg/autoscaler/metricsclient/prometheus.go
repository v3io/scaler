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
	"sync"
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

// metricResult holds the result of a single metric query for a resource
type metricResult struct {
	resourceName   string
	fullMetricName string
	metricValue    int
}

// PrometheusMetricsClient implements MetricsClient interface using Prometheus as the backend
type PrometheusMetricsClient struct {
	logger         logger.Logger
	apiClient      prometheusv1.API
	namespace      string
	queryTemplates map[string]*template.Template
	interval       time.Duration
}

// NewPrometheusClient creates a new PrometheusMetricsClient instance
func NewPrometheusClient(parentLogger logger.Logger, prometheusURL, namespace string, templates []scalertypes.QueryTemplate, interval time.Duration) (*PrometheusMetricsClient, error) {
	if len(templates) == 0 {
		return nil, errors.New("query templates cannot be empty")
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
		interval:       interval,
	}, nil
}

// GetResourceMetrics retrieves metrics for multiple resources
func (pc *PrometheusMetricsClient) GetResourceMetrics(resources []scalertypes.Resource) (map[string]map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pc.interval)
	defer cancel()
	metricToWindowSizes := pc.buildMetricLookup(resources)

	return pc.getResourceMetrics(ctx, metricToWindowSizes)
}

func (pc *PrometheusMetricsClient) getResourceMetrics(ctx context.Context, metricToWindowSizes metricLookup) (map[string]map[string]int, error) {
	metricsByResource := make(map[string]map[string]int)

	resultChan := make(chan *metricResult)
	wg := sync.WaitGroup{}

	for metricName, queryTemplate := range pc.queryTemplates {
		windowSizeToResources := metricToWindowSizes[metricName]

		// maximum goroutines is limited to the number of unique windowSize values across all metrics (5 by default).
		for windowSize, resourcesInWindowSize := range windowSizeToResources {
			wg.Add(1)
			go func(resourcesInWindowSize map[string]struct{}, metricName, windowSize string, resultChan chan<- *metricResult) {
				defer wg.Done()
				// create resource name regex for Prometheus query based on the resources in this window size
				resourceNameRegex := pc.createResourceNameRegex(resourcesInWindowSize)
				fullMetricName := scalertypes.GetKubernetesMetricName(metricName, windowSize)
				query, err := pc.renderQuery(queryTemplate, windowSize, resourceNameRegex)
				if err != nil {
					pc.logger.WarnWith("Failed to render query, skipping",
						"metricName", metricName,
						"windowSize", windowSize,
						"error", err.Error())
					return
				}

				rawResult, warnings, err := pc.apiClient.Query(ctx, query, time.Now())
				if err != nil {
					pc.logger.WarnWith("Failed to execute Prometheus query, skipping",
						"metricName", metricName,
						"windowSize", windowSize,
						"error", err.Error())
					return
				}

				if len(warnings) > 0 {
					pc.logger.WarnWith("Prometheus query returned warnings",
						"metricName", metricName,
						"windowSize", windowSize,
						"warnings", warnings)
				}

				metricSamples, ok := rawResult.(model.Vector)
				if !ok {
					pc.logger.WarnWith("Unexpected Prometheus result type, skipping",
						"metricName", metricName,
						"windowSize", windowSize)
					return
				}

				for _, metricSample := range metricSamples {
					resourceName, err := pc.extractResourceName(metricSample.Metric)
					if err != nil {
						pc.logger.WarnWith("Failed to extract resource name from prometheus metric labels, skipping",
							"metricName", metricName,
							"windowSize", windowSize,
							"error", err.Error())
						continue
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

					pc.logger.DebugWith("Retrieved metric",
						"resourceName", resourceName,
						"metricName", fullMetricName,
						"windowSize", windowSize,
						"value", metricValue)

					// finished processing this metric sample, send the result
					resultChan <- &metricResult{
						resourceName:   resourceName,
						fullMetricName: fullMetricName,
						metricValue:    metricValue,
					}
				}
			}(resourcesInWindowSize, metricName, windowSize, resultChan)
		}
	}

	var collectorErr error
	collectorDone := make(chan struct{})
	// Collect results
	go func(resultChan chan *metricResult) {
		defer close(collectorDone)
		for result := range resultChan {
			if _, exists := metricsByResource[result.resourceName]; !exists {
				metricsByResource[result.resourceName] = make(map[string]int)
			}
			if existingValue, exists := metricsByResource[result.resourceName][result.fullMetricName]; exists {
				if existingValue == result.metricValue {
					continue
				}
				collectorErr = errors.Errorf("conflicting metric values for resource: resourceName=%s, metricName=%s, existingValue=%d, newValue=%d",
					result.resourceName, result.fullMetricName, existingValue, result.metricValue)
				return
			}
			metricsByResource[result.resourceName][result.fullMetricName] = result.metricValue
		}
	}(resultChan)

	// wait for all queries to complete
	wg.Wait()
	close(resultChan)
	// wait for collector to finish processing results
	<-collectorDone

	if collectorErr != nil {
		return nil, collectorErr
	}

	if len(metricsByResource) == 0 {
		return nil, errors.New("no metrics retrieved for any resource")
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
	return "", errors.Errorf("could not extract resource name from labels: %v", labels)
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
