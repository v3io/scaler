package autoscaler

import (
	"time"

	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
)

type resourceMetricTypeMap map[string]map[string][]metricEntry

type metricEntry struct {
	timestamp    time.Time
	value        int
	resourceName string
	metricName   string
}

type MetricReporter interface {
	ReportMetric(metricEntry) error
}

type Autoscaler struct {
	logger         logger.Logger
	namespace      string
	metricsChannel chan metricEntry
	metricsMap     resourceMetricTypeMap
	resourceScaler scaler_types.ResourceScaler
	scaleInterval  time.Duration
}

func NewAutoScaler(parentLogger logger.Logger,
	resourceScaler scaler_types.ResourceScaler,
	options scaler_types.AutoScalerOptions) (*Autoscaler, error) {
	childLogger := parentLogger.GetChild("autoscale")
	childLogger.DebugWith("Creating Autoscaler",
		"options", options)

	return &Autoscaler{
		logger:         childLogger,
		namespace:      options.Namespace,
		metricsMap:     make(resourceMetricTypeMap),
		resourceScaler: resourceScaler,
		scaleInterval:  options.ScaleInterval,
		metricsChannel: make(chan metricEntry, 1024),
	}, nil
}

func (as *Autoscaler) checkResourceToScale(t time.Time, activeResources []scaler_types.Resource) {
	for _, resource := range activeResources {
		shouldScaleToZero := false
		for _, scaleResource := range resource.ScaleResources {
			oldestEntry := as.getOldestBelowThresholdMetricEntry(resource.Name, scaleResource)
			shouldScaleToZero = as.shouldScaleToZero(t, resource.Name, scaleResource, oldestEntry)

			// if one metric does not point that we should scale to zero - scale to zero won't happen,
			// so no need to check the other metrics
			if !shouldScaleToZero {
				break
			}
		}
		if shouldScaleToZero {
			err := as.resourceScaler.SetScale(resource, 0)
			if err != nil {
				as.logger.WarnWith("Failed to set scale", "err", err)
			}
		}
		as.cleanMetrics(t, resource, shouldScaleToZero)
	}
}

func (as *Autoscaler) getOldestBelowThresholdMetricEntry(resourceName string, scaleResource scaler_types.ScaleResource) *metricEntry {
	resourceMetrics := as.metricsMap[resourceName][scaleResource.MetricName]

	var oldestBelowThresholdMetricEntry *metricEntry
	for idx, metricEntry := range resourceMetrics {

		if metricEntry.value <= scaleResource.Threshold && oldestBelowThresholdMetricEntry == nil {
			oldestBelowThresholdMetricEntry = &resourceMetrics[idx]
		} else if metricEntry.value > scaleResource.Threshold {
			oldestBelowThresholdMetricEntry = nil
		}
	}

	return oldestBelowThresholdMetricEntry
}

func (as *Autoscaler) shouldScaleToZero(t time.Time, resourceName string, scaleResource scaler_types.ScaleResource, oldestEntry *metricEntry) bool {
	if oldestEntry != nil && t.Sub(oldestEntry.timestamp) > scaleResource.WindowSize {
		as.logger.DebugWith("Metric value is below threshold and passed the window",
			"metricValue", oldestEntry.value,
			"resourceName", resourceName,
			"metricName", scaleResource.MetricName,
			"threshold", scaleResource.Threshold,
			"deltaSeconds", t.Sub(oldestEntry.timestamp).Seconds(),
			"windowSize", scaleResource.WindowSize)
		return true
	} else if oldestEntry != nil {
		as.logger.DebugWith("Resource values are still in window",
			"resourceName", resourceName,
			"metricName", scaleResource.MetricName,
			"value", oldestEntry.value,
			"deltaSeconds", t.Sub(oldestEntry.timestamp).Seconds(),
			"windowSize", scaleResource.WindowSize)
		return false
	} else {
		as.logger.DebugWith("Resource metrics are above threshold",
			"resourceName", resourceName,
			"metricName", scaleResource.MetricName,
			"threshold", scaleResource.Threshold)
		return false
	}
}

func (as *Autoscaler) cleanMetrics(t time.Time, resource scaler_types.Resource, scaledToZero bool) {

	// If scale to zero occurred, metrics are not needed anymore
	if scaledToZero {
		delete(as.metricsMap, resource.Name)
	} else {
		for _, scaleResource := range resource.ScaleResources {

			// rebuild the slice, excluding any old metrics
			var newMetrics []metricEntry
			for _, metric := range as.metricsMap[resource.Name][scaleResource.MetricName] {
				if t.Sub(metric.timestamp) <= scaleResource.WindowSize {
					newMetrics = append(newMetrics, metric)
				}
			}

			if _, found := as.metricsMap[resource.Name]; !found {
				as.metricsMap[resource.Name] = make(map[string][]metricEntry)
			}

			as.metricsMap[resource.Name][scaleResource.MetricName] = newMetrics
		}
	}
}

func (as *Autoscaler) addMetricEntry(resourceName string, metricType string, entry metricEntry) {
	if _, found := as.metricsMap[resourceName]; !found {
		as.metricsMap[resourceName] = make(map[string][]metricEntry)
	}
	as.metricsMap[resourceName][metricType] = append(as.metricsMap[resourceName][metricType], entry)
}

func (as *Autoscaler) ReportMetric(metric metricEntry) error {

	// don't block, try and fail fast
	select {
	case as.metricsChannel <- metric:
		return nil
	default:
		as.logger.WarnWith("Failed to report metric",
			"resourceName", metric.resourceName,
			"MetricName", metric.metricName)
	}
	return nil
}

func (as *Autoscaler) Start() error {
	ticker := time.NewTicker(as.scaleInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				resourcesList, err := as.resourceScaler.GetResources()
				if err != nil {
					as.logger.WarnWith("Failed to build resource map", "err", err)
				}
				as.checkResourceToScale(time.Now(), resourcesList)
			case metric := <-as.metricsChannel:
				as.addMetricEntry(metric.resourceName, metric.metricName, metric)
			}
		}
	}()
	return nil
}
