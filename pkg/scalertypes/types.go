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
	"time"

	"github.com/nuclio/errors"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AutoScalerOptions struct {
	Namespace     string
	ScaleInterval Duration
	GroupKind     schema.GroupKind
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

func (sr ScaleResource) GetKubernetesMetricName() string {
	return fmt.Sprintf("%s_per_%s", sr.MetricName, shortDurationString(sr.WindowSize))
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

func shortDurationString(d Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
