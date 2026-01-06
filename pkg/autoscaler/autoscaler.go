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

package autoscaler

import (
	"time"

	"github.com/v3io/scaler/pkg/common"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Autoscaler struct {
	logger                  logger.Logger
	namespace               string
	resourceScaler          scalertypes.ResourceScaler
	scaleInterval           scalertypes.Duration
	inScaleToZeroProcessMap map[string]bool
	groupKind               schema.GroupKind
	metricsClient           scalertypes.MetricsClient
	ticker                  *time.Ticker
}

func NewAutoScaler(parentLogger logger.Logger,
	resourceScaler scalertypes.ResourceScaler,
	metricsClient scalertypes.MetricsClient,
	options scalertypes.AutoScalerOptions) (*Autoscaler, error) {
	childLogger := parentLogger.GetChild("autoscaler")
	childLogger.InfoWith("Creating Autoscaler",
		"options", options)

	return &Autoscaler{
		logger:                  childLogger,
		namespace:               options.Namespace,
		resourceScaler:          resourceScaler,
		scaleInterval:           options.ScaleInterval,
		groupKind:               options.GroupKind,
		metricsClient:           metricsClient,
		inScaleToZeroProcessMap: make(map[string]bool),
	}, nil
}

func (as *Autoscaler) Start() error {
	as.logger.DebugWith("Starting", "scaleInterval", as.scaleInterval)
	as.ticker = time.NewTicker(as.scaleInterval.Duration)
	go func() {
		for range as.ticker.C {
			if err := as.checkResourcesToScale(); err != nil {
				as.logger.WarnWith("Failed to check resources to scale",
					"err", errors.GetErrorStackString(err, 10))
			}
		}
		as.logger.Debug("Stopped ticking")
		as.ticker = nil
	}()
	return nil
}

func (as *Autoscaler) Stop() error {
	if as.ticker != nil {
		as.logger.Debug("Stopping")
		as.ticker.Stop()
	}
	return nil
}

func (as *Autoscaler) getMetricNames(resources []scalertypes.Resource) []string {
	var metricNames []string
	for _, resource := range resources {
		for _, scaleResource := range resource.ScaleResources {
			metricNames = append(metricNames, scaleResource.GetKubernetesMetricName())
		}
	}
	metricNames = common.UniquifyStringSlice(metricNames)
	return metricNames
}

func (as *Autoscaler) checkResourceToScale(resource scalertypes.Resource, resourcesMetricsMap map[string]map[string]int) bool {
	if _, found := resourcesMetricsMap[resource.Name]; !found {
		as.logger.DebugWith("Resource does not have metrics data yet, keeping up", "resourceName", resource.Name)
		return false
	}

	for _, scaleResource := range resource.ScaleResources {
		metricName := scaleResource.GetKubernetesMetricName()
		value, found := resourcesMetricsMap[resource.Name][metricName]
		if !found {
			as.logger.DebugWith("One of the metrics is missing data, keeping up",
				"resourceName", resource.Name,
				"metricName", metricName)
			return false
		}

		// Metric value above threshold, keeping up
		if value > scaleResource.Threshold {
			return false
		}

		as.logger.DebugWith("Metric value below threshold",
			"resourceName", resource.Name,
			"metricName", metricName,
			"threshold", scaleResource.Threshold,
			"value", value)
	}

	as.logger.DebugWith("All metric values below threshold, should scale to zero", "resourceName", resource.Name)
	return true
}

func (as *Autoscaler) getMaxScaleResourceWindowSize(resource scalertypes.Resource) time.Duration {
	maxWindow := 0 * time.Second
	for _, scaleResource := range resource.ScaleResources {
		if scaleResource.WindowSize.Duration > maxWindow {
			maxWindow = scaleResource.WindowSize.Duration
		}
	}
	return maxWindow
}

func (as *Autoscaler) checkResourcesToScale() error {
	now := time.Now()
	activeResources, err := as.resourceScaler.GetResources()
	if err != nil {
		return errors.Wrap(err, "Failed to get resources")
	}
	if len(activeResources) == 0 {
		return nil
	}
	metricNames := as.getMetricNames(activeResources)
	as.logger.DebugWith("Got metric names", "metricNames", metricNames)
	resourceMetricsMap, err := as.metricsClient.GetResourceMetrics(metricNames)
	if err != nil {
		return errors.Wrap(err, "Failed to get resources metrics")
	}

	resourcesToScale := make([]scalertypes.Resource, 0)
	for idx, resource := range activeResources {
		inScaleToZeroProcess, found := as.inScaleToZeroProcessMap[resource.Name]
		if found && inScaleToZeroProcess {
			as.logger.DebugWith("Already in scale to zero process, skipping",
				"resourceName", resource.Name)
			continue
		}

		scaleEventDebounceDuration := as.getMaxScaleResourceWindowSize(resource)

		// if the resource was scaled from zero or updated, and the debounce period from then has not passed yet do not scale
		if ((resource.LastScaleEvent != nil) &&
			(*resource.LastScaleEvent == scalertypes.ResourceUpdatedScaleEvent ||
				*resource.LastScaleEvent == scalertypes.ScaleFromZeroStartedScaleEvent ||
				*resource.LastScaleEvent == scalertypes.ScaleFromZeroCompletedScaleEvent)) &&
			resource.LastScaleEventTime.After(now.Add(-1*scaleEventDebounceDuration)) {
			as.logger.DebugWith("Resource in debouncing period, not a scale-to-zero candidate",
				"resourceName", resource.Name,
				"LastScaleEvent", *resource.LastScaleEvent,
				"LastScaleEventTime", *resource.LastScaleEventTime,
				"scaleEventDebounceDuration", scaleEventDebounceDuration,
				"time", now)
			continue
		}

		shouldScaleToZero := as.checkResourceToScale(resource, resourceMetricsMap)

		if !shouldScaleToZero {
			continue
		}

		as.inScaleToZeroProcessMap[resource.Name] = true
		resourcesToScale = append(resourcesToScale, activeResources[idx])
	}

	if len(resourcesToScale) > 0 {
		go func(resources []scalertypes.Resource) {
			as.logger.InfoWith("Scaling resources to zero", "resources", resources)
			if err := as.scaleResourcesToZero(resources); err != nil {
				as.logger.WarnWith("Failed to scale resources to zero",
					"resources", resources,
					"err", errors.GetErrorStackString(err, 10))
			}
			as.logger.InfoWith("Successfully scaled resources to zero", "resources", resources)
			for _, resource := range resources {
				delete(as.inScaleToZeroProcessMap, resource.Name)
			}
		}(resourcesToScale)
	}

	return nil
}

func (as *Autoscaler) scaleResourcesToZero(resources []scalertypes.Resource) error {
	if err := as.resourceScaler.SetScale(resources, 0); err != nil {
		return errors.Wrap(err, "Failed to set scale")
	}

	return nil
}
