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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/suite"
)

const (
	testQueryTemplateWithResources = `sum(rate(handled_events_total{namespace="{{ .Namespace }}", trigger_kind="http", function=~"{{ .Resources }}"}[{{ .WindowSize }}])) by (function)`
)

type PrometheusClientTestSuite struct {
	suite.Suite
	logger logger.Logger
}

func (suite *PrometheusClientTestSuite) SetupTest() {
	var err error
	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
}

// createMockPrometheusServer creates an HTTP test server with a custom response result
func createMockPrometheusServer(serverResult []map[string]interface{}) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/query":
			timestamp := float64(time.Now().Unix())

			// Update timestamps in serverResult
			for _, result := range serverResult {
				if value, ok := result["value"].([]interface{}); ok && len(value) > 1 {
					result["value"] = []interface{}{timestamp, value[1]}
				}
			}

			response := map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"resultType": "vector",
					"result":     serverResult,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(handler)
}

func (suite *PrometheusClientTestSuite) TestGetResourceMetrics() {
	timestamp := float64(time.Now().Unix())
	tests := []struct {
		name           string
		resources      []scalertypes.Resource
		serverResult   []map[string]interface{}
		expectedResult map[string]map[string]int
	}{
		{
			name: "single resource with metrics",
			resources: []scalertypes.Resource{
				{
					Name: "test-resource1",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
			},
			serverResult: []map[string]interface{}{
				{
					"metric": map[string]string{
						"function": "test-resource1",
					},
					"value": []interface{}{timestamp, "0"},
				},
			},
			expectedResult: map[string]map[string]int{
				"test-resource1": {
					"handled_events_total_per_1m": 0,
				},
			},
		},
		{
			name: "multiple resources with same window size",
			resources: []scalertypes.Resource{
				{
					Name: "test-resource1",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
				{
					Name: "test-resource3",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
			},
			serverResult: []map[string]interface{}{
				{
					"metric": map[string]string{
						"function": "test-resource1",
					},
					"value": []interface{}{timestamp, "0"},
				},
				{
					"metric": map[string]string{
						"function": "test-resource3",
					},
					"value": []interface{}{timestamp, "0.5"},
				},
			},
			expectedResult: map[string]map[string]int{
				"test-resource1": {
					"handled_events_total_per_1m": 0,
				},
				"test-resource3": {
					"handled_events_total_per_1m": 1,
				},
			},
		}, {
			name: "multiple resources with different window size",
			resources: []scalertypes.Resource{
				{
					Name: "test-resource1",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
				{
					Name: "test-resource3",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 3 * time.Minute},
						},
					},
				},
			},
			serverResult: []map[string]interface{}{
				{
					"metric": map[string]string{
						"function": "test-resource1",
					},
					"value": []interface{}{timestamp, "0.1"},
				},
				{
					"metric": map[string]string{
						"function": "test-resource3",
					},
					"value": []interface{}{timestamp, "0.1"},
				},
			},
			expectedResult: map[string]map[string]int{
				"test-resource1": {
					"handled_events_total_per_1m": 1,
					"handled_events_total_per_3m": 1,
				},
				"test-resource3": {
					"handled_events_total_per_1m": 1,
					"handled_events_total_per_3m": 1,
				},
			},
		},
	}

	for _, testCase := range tests {
		suite.Run(testCase.name, func() {
			mockServer := createMockPrometheusServer(testCase.serverResult)
			defer mockServer.Close()

			// Create Prometheus client pointing to mock server
			// the client creation is done inside the test because each test have a unique mock server URL
			client, err := NewPrometheusClient(suite.logger,
				mockServer.URL,
				"test-namespace",
				[]scalertypes.QueryTemplate{
					{
						Name:     "handled_events_total",
						Template: testQueryTemplateWithResources,
					},
				},
			)
			suite.Require().NoError(err)

			results, err := client.GetResourceMetrics(testCase.resources)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, results, "GetResourceMetrics() result mismatch")
		})
	}
}

func (suite *PrometheusClientTestSuite) TestRenderQuery() {
	tests := []struct {
		name              string
		namespace         string
		templateStr       string
		windowSize        string
		resourceNameRegex string
		expectedQuery     string
		expectError       bool
	}{
		{
			name:              "basic query with all fields",
			namespace:         "test-namespace",
			templateStr:       testQueryTemplateWithResources,
			windowSize:        "1m",
			resourceNameRegex: "test-resource1|test-resource2",
			expectedQuery:     `sum(rate(handled_events_total{namespace="test-namespace", trigger_kind="http", function=~"test-resource1|test-resource2"}[1m])) by (function)`,
			expectError:       false,
		},
		{
			name:              "query with different namespace",
			namespace:         "my-namespace",
			templateStr:       `sum(rate(handled_events_total{namespace="{{ .Namespace }}"}[{{ .WindowSize }}])) by (function)`,
			windowSize:        "5m",
			resourceNameRegex: "resource1",
			expectedQuery:     `sum(rate(handled_events_total{namespace="my-namespace"}[5m])) by (function)`,
			expectError:       false,
		},
		{
			name:              "query with single resource",
			namespace:         "test-namespace",
			templateStr:       testQueryTemplateWithResources,
			windowSize:        "2m",
			resourceNameRegex: "test-resource1",
			expectedQuery:     `sum(rate(handled_events_total{namespace="test-namespace", trigger_kind="http", function=~"test-resource1"}[2m])) by (function)`,
			expectError:       false,
		},
		{
			name:              "query with no resource regex",
			namespace:         "test-namespace",
			templateStr:       `sum(rate(handled_events_total{namespace="{{ .Namespace }}"}[{{ .WindowSize }}])) by (function)`,
			windowSize:        "1m",
			resourceNameRegex: "",
			expectedQuery:     `sum(rate(handled_events_total{namespace="test-namespace"}[1m])) by (function)`,
			expectError:       false,
		},
		{
			name:              "query with special characters in resource names",
			namespace:         "test-namespace",
			templateStr:       `sum(rate(handled_events_total{namespace="{{ .Namespace }}", function=~"{{ .Resources }}"}[{{ .WindowSize }}])) by (function)`,
			windowSize:        "30m",
			resourceNameRegex: "function-with-long-stz|hello-world",
			expectedQuery:     `sum(rate(handled_events_total{namespace="test-namespace", function=~"function-with-long-stz|hello-world"}[30m])) by (function)`,
			expectError:       false,
		},
		{
			name:              "invalid template syntax",
			namespace:         "test-namespace",
			templateStr:       `sum(rate(handled_events_total{namespace="{{ .Namespace }}"}[{{ .WindowSize }}])) by (function){{ .InvalidField }}`,
			windowSize:        "1m",
			resourceNameRegex: "test-resource1",
			expectedQuery:     "",
			expectError:       false, // Template execution doesn't fail on missing fields, just renders empty
		},
	}

	for _, testCase := range tests {
		suite.Run(testCase.name, func() {
			client := &PrometheusMetricsClient{
				namespace: testCase.namespace,
			}

			tmpl, err := template.New("test").Parse(testCase.templateStr)
			suite.Require().NoError(err, "Failed to parse template")
			result, err := client.renderQuery(tmpl, testCase.windowSize, testCase.resourceNameRegex)

			if testCase.expectError {
				suite.Require().Error(err, "Expected error but got none")
			} else {
				suite.Require().NoError(err, "Unexpected error")
				if testCase.expectedQuery != "" {
					suite.Require().Equal(testCase.expectedQuery, result, "Query mismatch")
				}
			}
		})
	}
}

func (suite *PrometheusClientTestSuite) TestExtractWindowSizesForMetric() {
	tests := []struct {
		name       string
		resources  []scalertypes.Resource
		metricName string
		expected   map[string]bool
	}{
		{
			name: "single resource with single window size",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"1m": true},
		},
		{
			name: "multiple resources with same window size",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
				{
					Name:      "test-resource2",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"1m": true},
		},
		{
			name: "multiple resources with different window sizes",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
				{
					Name:      "test-resource2",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 2 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"1m": true, "2m": true},
		},
		{
			name: "resource with multiple scale resources for same metric",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 5 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"1m": true, "5m": true},
		},
		{
			name: "resources with different metric names",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
						{
							MetricName: "other_metric",
							WindowSize: scalertypes.Duration{Duration: 2 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"1m": true},
		},
		{
			name: "no resources with specified metric name",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "other_metric",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Minute},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{},
		},
		{
			name:       "empty resources",
			resources:  []scalertypes.Resource{},
			metricName: "handled_events_total",
			expected:   map[string]bool{},
		},
		{
			name: "window sizes with different formats",
			resources: []scalertypes.Resource{
				{
					Name:      "test-resource1",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				{
					Name:      "test-resource2",
					Namespace: "test-namespace",
					ScaleResources: []scalertypes.ScaleResource{
						{
							MetricName: "handled_events_total",
							WindowSize: scalertypes.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			metricName: "handled_events_total",
			expected:   map[string]bool{"30m": true, "1h": true},
		},
	}

	for _, testCase := range tests {
		suite.Run(testCase.name, func() {
			client := &PrometheusMetricsClient{
				namespace: "test-namespace",
			}

			result := client.extractWindowSizesForMetric(testCase.resources, testCase.metricName)

			suite.Require().Equal(len(testCase.expected), len(result), "Window sizes count mismatch")
			for expectedKey := range testCase.expected {
				suite.Require().True(result[expectedKey], "Expected window size %s not found", expectedKey)
			}
			for resultKey := range result {
				suite.Require().True(testCase.expected[resultKey], "Unexpected window size %s found", resultKey)
			}
		})
	}
}

func TestPrometheusClientSuite(t *testing.T) {
	suite.Run(t, new(PrometheusClientTestSuite))
}
