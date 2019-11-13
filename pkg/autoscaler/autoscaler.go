package autoscaler

import (
	"time"

	"github.com/v3io/scaler/pkg/common"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	custommetricsv1 "k8s.io/metrics/pkg/client/custom_metrics"
)

type Autoscaler struct {
	logger                  logger.Logger
	namespace               string
	resourceScaler          scaler_types.ResourceScaler
	scaleInterval           time.Duration
	inScaleToZeroProcessMap map[string]bool
	groupKind               string
	customMetricsClientSet  custommetricsv1.CustomMetricsClient
}

func NewAutoScaler(parentLogger logger.Logger,
	resourceScaler scaler_types.ResourceScaler,
	customMetricsClientSet custommetricsv1.CustomMetricsClient,
	options scaler_types.AutoScalerOptions) (*Autoscaler, error) {
	childLogger := parentLogger.GetChild("autoscaler")
	childLogger.DebugWith("Creating Autoscaler",
		"options", options)

	return &Autoscaler{
		logger:                  childLogger,
		namespace:               options.Namespace,
		resourceScaler:          resourceScaler,
		scaleInterval:           options.ScaleInterval,
		groupKind:               options.GroupKind,
		customMetricsClientSet:  customMetricsClientSet,
		inScaleToZeroProcessMap: make(map[string]bool),
	}, nil
}

func (as *Autoscaler) Start() error {
	ticker := time.NewTicker(as.scaleInterval)

	go func() {
		for range ticker.C {
			err := as.checkResourcesToScale()
			if err != nil {
				as.logger.WarnWith("Failed to check resources to scale", "err", errors.GetErrorStackString(err, 10))
			}
		}
	}()
	return nil
}

func (as *Autoscaler) getMetricNames(resources []scaler_types.Resource) []string {
	var metricNames []string
	for _, resource := range resources {
		for _, scaleResource := range resource.ScaleResources {
			metricNames = append(metricNames, scaleResource.GetKubernetesMetricName())
		}
	}
	metricNames = common.UniquifyStringSlice(metricNames)
	return metricNames
}

func (as *Autoscaler) getResourceMetrics(metricNames []string) (map[string]map[string]int, error) {
	resourcesMetricsMap := make(map[string]map[string]int)

	schemaGroupKind := schema.GroupKind{Group: "", Kind: as.groupKind}
	resourceLabels := labels.Everything()
	metricsClient := as.customMetricsClientSet.NamespacedMetrics(as.namespace)

	for _, metricName := range metricNames {

		// getting the metric values for all object of schema group kind (e.g. deployment)
		metrics, err := metricsClient.GetForObjects(schemaGroupKind,
			resourceLabels,
			metricName)
		if err != nil {

			// if no data points submitted yet it's ok, continue to the next metric
			if k8serrors.IsNotFound(err) {
				continue
			}
			return make(map[string]map[string]int), errors.Wrap(err, "Failed to get custom metrics")
		}

		// fill the resourcesMetricsMap with the metrics data we got
		for _, item := range metrics.Items {

			resourceName := item.DescribedObject.Name
			value := int(item.Value.MilliValue())

			as.logger.DebugWith("Got metric entry",
				"resourceName", resourceName,
				"metricName", metricName,
				"value", value)

			if _, found := resourcesMetricsMap[resourceName]; !found {
				resourcesMetricsMap[resourceName] = make(map[string]int)
			}

			// sanity
			if _, found := resourcesMetricsMap[resourceName][metricName]; found {
				return make(map[string]map[string]int), errors.New("Can not have more than one metric value per resource")
			}

			resourcesMetricsMap[resourceName][metricName] = value
		}
	}

	return resourcesMetricsMap, nil
}

func (as *Autoscaler) checkResourceToScale(resource scaler_types.Resource, resourcesMetricsMap map[string]map[string]int) bool {
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

	as.logger.InfoWith("All metric values below threshold, should scale to zero", "resourceName", resource.Name)
	return true
}

func (as *Autoscaler) getMaxScaleResourceWindowSize(resource scaler_types.Resource) time.Duration {
	maxWindow := 0 * time.Second
	for _, scaleResource := range resource.ScaleResources {
		if scaleResource.WindowSize > maxWindow {
			maxWindow = scaleResource.WindowSize
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
	resourceMetricsMap, err := as.getResourceMetrics(metricNames)
	if err != nil {
		return errors.Wrap(err, "Failed to get resources metrics")
	}

	for idx, resource := range activeResources {
		inScaleToZeroProcess, found := as.inScaleToZeroProcessMap[resource.Name]
		if found && inScaleToZeroProcess {
			as.logger.DebugWith("Already in scale to zero process, skipping",
				"resourceName", resource.Name)
			continue
		}

		scaleEventDebounceDuration := as.getMaxScaleResourceWindowSize(resource)

		// if the resource was scaled from zero or started, and the debounce period from then has not passed yet do not scale
		if ((resource.LastScaleEvent != nil) &&
			(*resource.LastScaleEvent == scaler_types.ResourceStartedScaleEvent ||
				*resource.LastScaleEvent == scaler_types.ScaleFromZeroStartedScaleEvent ||
				*resource.LastScaleEvent == scaler_types.ScaleFromZeroCompletedScaleEvent)) &&
			resource.LastScaleEventTime.After(now.Add(-1*scaleEventDebounceDuration)) {
			as.logger.DebugWith("Resource in debouncing period, not a scale-to-zero candidate",
				"resourceName", resource.Name,
				"LastScaleEventTime", resource.LastScaleEventTime,
				"scaleEventDebounceDuration", scaleEventDebounceDuration,
				"time", now)
			continue
		}

		shouldScaleToZero := as.checkResourceToScale(resource, resourceMetricsMap)

		if !shouldScaleToZero {
			continue
		}

		as.inScaleToZeroProcessMap[resource.Name] = true
		go func(resource scaler_types.Resource) {
			err := as.scaleResourceToZero(resource)
			if err != nil {
				as.logger.WarnWith("Failed to scale resource to zero", "resource", resource, "err", errors.GetErrorStackString(err, 10))
			}
			delete(as.inScaleToZeroProcessMap, resource.Name)
		}(activeResources[idx])
	}
	return nil
}

func (as *Autoscaler) scaleResourceToZero(resource scaler_types.Resource) error {
	if err := as.resourceScaler.SetScale(resource, 0); err != nil {
		return errors.Wrap(err, "Failed to set scale")
	}

	return nil
}
