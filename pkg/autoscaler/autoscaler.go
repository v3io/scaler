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
		for {
			select {
			case <-ticker.C:
				err := as.checkResourcesToScale(time.Now())
				if err != nil {
					as.logger.WarnWith("Failed to check resources to scale", "err", errors.GetErrorStackString(err, 10))
				}
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
	metricNames = common.UniquifyStringList(metricNames)
	as.logger.DebugWith("Got metric names", "metricNames", metricNames)
	return metricNames
}

func (as *Autoscaler) getResourcesMetrics(metricNames []string) (map[string]map[string]int, error) {
	resourcesMetricsMap := make(map[string]map[string]int)

	schemaGroupKind := schema.GroupKind{Group: "", Kind: as.groupKind}
	resourceLabels := labels.Everything()
	c := as.customMetricsClientSet.NamespacedMetrics(as.namespace)

	for _, metricName := range metricNames {
		cm, err := c.GetForObjects(schemaGroupKind,
			resourceLabels,
			metricName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return make(map[string]map[string]int), errors.Wrap(err, "Failed to get custom metrics")
		}

		for _, item := range cm.Items {

			resourceName := item.DescribedObject.Name
			value := int(item.Value.MilliValue())

			as.logger.DebugWith("Got Metric Entry",
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

		if value > scaleResource.Threshold {
			as.logger.DebugWith("Metric value above threshold, keeping up",
				"resourceName", resource.Name,
				"metricName", metricName,
				"threshold", scaleResource.Threshold,
				"value", value)
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

func (as *Autoscaler) getBiggestWindow(resource scaler_types.Resource) time.Duration {
	biggestWindow := 0 * time.Second
	for _, scaleResource := range resource.ScaleResources {
		if scaleResource.WindowSize > biggestWindow {
			biggestWindow = scaleResource.WindowSize
		}
	}
	return biggestWindow
}

func (as *Autoscaler) checkResourcesToScale(t time.Time) error {
	activeResources, err := as.resourceScaler.GetResources()
	if err != nil {
		return errors.Wrap(err, "Failed to get resources")
	}
	if len(activeResources) == 0 {
		return nil
	}
	metricNames := as.getMetricNames(activeResources)
	resourcesMetricsMap, err := as.getResourcesMetrics(metricNames)
	if err != nil {
		return errors.Wrap(err, "Failed to get resources metrics")
	}

	for _, resource := range activeResources {
		inScaleToZeroProcess, found := as.inScaleToZeroProcessMap[resource.Name]
		if found && inScaleToZeroProcess {
			as.logger.DebugWith("Already in scale to zero process, skipping",
				"resourceName", resource.Name)
			continue
		}

		biggestWindow := as.getBiggestWindow(resource)

		// if the resource was scaled from zero, and it happened after biggest window ago don't scale
		if (resource.LastScaleState == scaler_types.ScalingFromZeroScaleState ||
			resource.LastScaleState == scaler_types.ScaledFromZeroScaleState) &&
			resource.LastScaleStateTime.After(t.Add(-1*biggestWindow)) {
			as.logger.DebugWith("Not enough time passed from last scale from zero event, keeping up",
				"resourceName", resource.Name,
				"lastScaleStateTime", resource.LastScaleStateTime,
				"biggestWindow", biggestWindow,
				"time", t)
			continue
		}

		shouldScaleToZero := as.checkResourceToScale(resource, resourcesMetricsMap)

		if !shouldScaleToZero {
			continue
		}

		as.inScaleToZeroProcessMap[resource.Name] = true
		go func() {
			err := as.scaleResourceToZero(resource)
			if err != nil {
				as.logger.WarnWith("Failed to scale resource to zero", "resource", resource, "err", errors.GetErrorStackString(err, 10))
			}
			delete(as.inScaleToZeroProcessMap, resource.Name)
		}()
	}
	return nil
}

func (as *Autoscaler) scaleResourceToZero(resource scaler_types.Resource) error {
	if err := as.resourceScaler.SetScale(resource, 0); err != nil {
		return errors.Wrap(err, "Failed to set scale")
	}

	return nil
}
