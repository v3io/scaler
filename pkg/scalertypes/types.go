/*
Copyright 2019 Iguazio Systems Ltd.

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

package scalertypes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/nuclio/errors"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

type MetricsClientKind string

const (
	KindK8sMetricsClient = "k8sMetricsClient"
	KindPrometheusClient = "prometheusClient"
)

// QueryTemplate defines a named Prometheus query template.
type QueryTemplate struct {
	Name     string
	Template string
}

// CreateQueryTemplate parses and validates the query template
func (q *QueryTemplate) CreateQueryTemplate() (*template.Template, error) {
	if q.Name == "" {
		return nil, errors.New("template name cannot be empty")
	}
	if q.Template == "" {
		return nil, errors.New("query template cannot be empty")
	}
	tmpl, err := template.New(q.Name).Parse(q.Template)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse query template")
	}
	return tmpl, nil
}

type MetricsClientOptions struct {
	MetricsClientKind MetricsClientKind
	URL               string
	QueryTemplates    []QueryTemplate
}

type AutoScalerOptions struct {
	Namespace            string
	ScaleInterval        Duration
	GroupKind            schema.GroupKind
	MetricsClientOptions MetricsClientOptions
}

type ResourceScalerConfig struct {
	KubeconfigPath    string
	AutoScalerOptions AutoScalerOptions
	DLXOptions        DLXOptions
}

type MultiTargetStrategy string

const (
	MultiTargetStrategyRandom  MultiTargetStrategy = "random"
	MultiTargetStrategyPrimary MultiTargetStrategy = "primary"
	MultiTargetStrategyCanary  MultiTargetStrategy = "canary"
)

const (
	DefaultResyncInterval = 30 * time.Second
)

// ResolveTargetsFromIngressCallback defines a function that extracts a list of target identifiers
// (e.g., names of services the Ingress routes traffic to) from a Kubernetes Ingress resource.
//
// This function is expected to be implemented externally and passed into the IngressWatcher,
// allowing for custom logic such as parsing annotations, labels, or other ingress metadata.
//
// Parameters:
//   - ingress: The Kubernetes Ingress resource to extract targets from
//
// Returns:
//   - []string: A slice of target identifiers (e.g., service names, endpoint addresses)
//   - error: An error if target resolution fails
//
// Implementation guidelines:
// - Return a non-nil slice when targets are successfully resolved
// - Return a non-nil error if resolution fails
// - Should handle nil or malformed Ingress objects gracefully and return an error in such cases
type ResolveTargetsFromIngressCallback func(ingress *networkingv1.Ingress) ([]string, error)

type DLXOptions struct {
	Namespace string

	// comma delimited
	TargetNameHeader                  string
	TargetPathHeader                  string
	TargetPort                        int
	ListenAddress                     string
	ResourceReadinessTimeout          Duration
	MultiTargetStrategy               MultiTargetStrategy
	LabelSelector                     string
	ResolveTargetsFromIngressCallback ResolveTargetsFromIngressCallback `json:"-"`
	ResyncInterval                    Duration
	KubeClientSet                     kubernetes.Interface `json:"-"`
}

type ResourceScaler interface {
	SetScale([]Resource, int) error
	SetScaleCtx(context.Context, []Resource, int) error
	GetResources() ([]Resource, error)
	GetConfig() (*ResourceScalerConfig, error)
	ResolveServiceName(Resource) (string, error)
}

type Resource struct {
	Name               string          `json:"name,omitempty"`
	Namespace          string          `json:"namespace,omitempty"`
	ScaleResources     []ScaleResource `json:"scale_resources,omitempty"`
	LastScaleEvent     *ScaleEvent     `json:"last_scale_event,omitempty"`
	LastScaleEventTime *time.Time      `json:"last_scale_event_time,omitempty"`
}

func (r Resource) String() string {
	out, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	return string(out)
}

type ScaleResource struct {
	MetricName string   `json:"metric_name,omitempty"`
	WindowSize Duration `json:"windows_size,omitempty"`
	Threshold  int      `json:"threshold,omitempty"`
}

// GetKubernetesMetricName constructs a Kubernetes metric name from a base metric name and window size
func (sr ScaleResource) GetKubernetesMetricName() string {
	return GetKubernetesMetricName(sr.MetricName, ShortDurationString(sr.WindowSize))
}

// GetKubernetesMetricName constructs a Kubernetes metric name from a base metric name and window size
func GetKubernetesMetricName(metricName, windowSize string) string {
	return fmt.Sprintf("%s_per_%s", metricName, windowSize)
}

func (sr ScaleResource) String() string {
	out, err := json.Marshal(sr)
	if err != nil {
		panic(err)
	}
	return string(out)
}

type ScaleEvent string

const (
	ResourceUpdatedScaleEvent        ScaleEvent = "resourceUpdated"
	ScaleFromZeroStartedScaleEvent   ScaleEvent = "scaleFromZeroStarted"
	ScaleFromZeroCompletedScaleEvent ScaleEvent = "scaleFromZeroCompleted"
	ScaleToZeroStartedScaleEvent     ScaleEvent = "scaleToZeroStarted"
	ScaleToZeroCompletedScaleEvent   ScaleEvent = "scaleToZeroCompleted"
)

func ParseScaleEvent(scaleEventStr string) (ScaleEvent, error) {
	switch scaleEventStr {
	case string(ResourceUpdatedScaleEvent):
		return ResourceUpdatedScaleEvent, nil
	case string(ScaleFromZeroStartedScaleEvent):
		return ScaleFromZeroStartedScaleEvent, nil
	case string(ScaleFromZeroCompletedScaleEvent):
		return ScaleFromZeroCompletedScaleEvent, nil
	case string(ScaleToZeroStartedScaleEvent):
		return ScaleToZeroStartedScaleEvent, nil
	case string(ScaleToZeroCompletedScaleEvent):
		return ScaleToZeroCompletedScaleEvent, nil
	default:
		return "", errors.Errorf("Unknown scale event: %s", scaleEventStr)
	}
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		if d.Duration, err = time.ParseDuration(value); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

// ShortDurationString formats a Duration into a short string representation by removing trailing zeros
func ShortDurationString(d Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// MetricsClient defines an interface for retrieving resource metrics used by the autoscaler.
type MetricsClient interface {
	// GetResourceMetrics retrieves metrics for multiple resources.
	//
	// Parameters:
	//   - resources: A slice of resources to retrieve metrics for
	//
	// Returns:
	//   - map[string]map[string]int: A nested map structure where:
	//     * The outer map key is the resource name (e.g., deployment name)
	//     * The inner map key is the metric name
	//     * The inner map value is the metric value as an integer
	//     Example: map["my-deployment"]["requests_per_minute"] = 42
	//   - error: An error if metric retrieval fails
	//
	// The dual map structure allows efficient lookup of metric values by resource name
	// and then by metric name, enabling the autoscaler to check multiple metrics
	// per resource when making scaling decisions.
	GetResourceMetrics(resources []Resource) (map[string]map[string]int, error)
}
