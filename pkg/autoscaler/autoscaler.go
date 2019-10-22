package autoscaler

import (
	"time"

	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
)

type resourceMetricTypeMap map[string]map[string][]metricEntry

type metricEntry struct {
	timestamp    time.Time
	value        int64
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
	threshold      int64
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
		threshold:      options.Threshold,
		metricsChannel: make(chan metricEntry, 1024),
	}, nil
}

func (as *Autoscaler) checkResourceToScale(t time.Time, activeResources []scaler_types.Resource) {
	for _, resource := range activeResources {
		shouldScaleToZero := false
		for _, metricName := range resource.MetricNames {
			oldestEntry := as.getOldestBelowThresholdMetricEntry(resource.Name, metricName)
			shouldScaleToZero = as.shouldScaleToZero(t, resource, oldestEntry)

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

func (as *Autoscaler) getOldestBelowThresholdMetricEntry(resourceName string, metricName string) *metricEntry {
	resourceMetrics := as.metricsMap[resourceName][metricName]

	var oldestBelowThresholdMetricEntry *metricEntry
	for _, metricEntry := range resourceMetrics {

		if metricEntry.value <= as.threshold && oldestBelowThresholdMetricEntry == nil {
			oldestBelowThresholdMetricEntry = &metricEntry
		} else if metricEntry.value > as.threshold {
			oldestBelowThresholdMetricEntry = nil
		}
	}

	return oldestBelowThresholdMetricEntry
}

func (as *Autoscaler) shouldScaleToZero(t time.Time, resource scaler_types.Resource, oldestEntry *metricEntry) bool {
	if oldestEntry != nil && t.Sub(oldestEntry.timestamp) > resource.WindowSize {
		as.logger.DebugWith("Metric value is below threshold and passed the window",
			"metricValue", oldestEntry.value,
			"resource", resource.Name,
			"deltaSeconds", t.Sub(oldestEntry.timestamp).Seconds(),
			"windowSize", resource.WindowSize)
		return true
	} else if oldestEntry != nil {
		as.logger.DebugWith("Resource values are still in window",
			"resourceName", resource.Name,
			"value", oldestEntry.value,
			"deltaSeconds", t.Sub(oldestEntry.timestamp).Seconds(),
			"windowSize", resource.WindowSize)
		return false
	} else {
		as.logger.Debug("Resource metrics are above threshold",
			"resourceName", resource.Name,
			"threshold", as.threshold)
		return false
	}
}

func (as *Autoscaler) cleanMetrics(t time.Time, resource scaler_types.Resource, scaledToZero bool) {

	// If scale to zero occurred, metrics are not needed anymore
	if scaledToZero {
		delete(as.metricsMap, resource.Name)
	} else {
		for _, metricName := range resource.MetricNames {

			// rebuild the slice, excluding any old metrics
			var newMetrics []metricEntry
			for _, metric := range as.metricsMap[resource.Name][metricName] {
				if t.Sub(metric.timestamp) <= resource.WindowSize {
					newMetrics = append(newMetrics, metric)
				}
			}
			as.metricsMap[resource.Name][metricName] = newMetrics
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
