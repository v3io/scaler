package autoscaler

import (
	"time"

	"github.com/v3io/scaler/pkg"

	"github.com/nuclio/logger"
)

type resourceMetricTypeMap map[scaler.Resource]map[string][]metricEntry

type metricEntry struct {
	timestamp    time.Time
	value        int64
	resourceName scaler.Resource
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
	resourceScaler scaler.ResourceScaler
	metricName     string
	scaleInterval  time.Duration
	windowSize     time.Duration
	threshold      int64
}

func NewAutoScaler(parentLogger logger.Logger, options scaler.AutoScalerOptions) (*Autoscaler, error) {
	childLogger := parentLogger.GetChild("autoscale")
	childLogger.DebugWith("Creating Autoscaler",
		"Namespace", options.Namespace,
		"MetricName", options.MetricName)

	return &Autoscaler{
		logger:         childLogger,
		namespace:      options.Namespace,
		metricsMap:     make(resourceMetricTypeMap),
		resourceScaler: options.ResourceScaler,
		metricName:     options.MetricName,
		windowSize:     options.ScaleWindow,
		scaleInterval:  options.ScaleInterval,
		threshold:      options.Threshold,
		metricsChannel: make(chan metricEntry, 1024),
	}, nil
}

func (as *Autoscaler) checkResourceToScale(t time.Time, activeResources []scaler.Resource) {
	for _, resourceName := range activeResources {

		// currently only one type of metric supported from a platform configuration
		resourceMetrics := as.metricsMap[resourceName][as.metricName]

		// this will give out the greatest delta
		var minMetric *metricEntry
		for idx, metric := range resourceMetrics {

			if metric.value <= as.threshold && minMetric == nil {
				minMetric = &resourceMetrics[idx]
			} else if metric.value > as.threshold {
				minMetric = nil
			}
		}

		if minMetric != nil && t.Sub(minMetric.timestamp) > as.windowSize {
			as.logger.DebugWith("Metric value is below threshold and passed the window",
				"metricValue", minMetric.value,
				"resource", resourceName,
				"deltaSeconds", t.Sub(minMetric.timestamp).Seconds(),
				"windowSize", as.windowSize)

			err := as.resourceScaler.SetScale(as.logger, as.namespace, resourceName, 0)
			if err != nil {
				as.logger.WarnWith("Failed to set scale", "err", err)
			}
			delete(as.metricsMap, resourceName)
		} else if as.metricsMap[resourceName][as.metricName] != nil {
			if minMetric != nil {
				as.logger.DebugWith("Resource values are still in window",
					"resourceName", resourceName,
					"value", minMetric.value,
					"deltaSeconds", t.Sub(minMetric.timestamp).Seconds(),
					"windowSize", as.windowSize)
			} else {
				as.logger.Debug("Resource metrics are above threshold")
			}

			// rebuild the slice, excluding any old metrics
			var newMetrics []metricEntry
			for _, metric := range resourceMetrics {
				if t.Sub(metric.timestamp) <= as.windowSize {
					newMetrics = append(newMetrics, metric)
				}
			}
			as.metricsMap[resourceName][as.metricName] = newMetrics
		}
	}
}

func (as *Autoscaler) addMetricEntry(resourceName scaler.Resource, metricType string, entry metricEntry) {
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
					as.logger.WarnWith("Failed to build resource map")
				}
				as.checkResourceToScale(time.Now(), resourcesList)
			case metric := <-as.metricsChannel:
				as.addMetricEntry(metric.resourceName, metric.metricName, metric)
			}
		}
	}()
	return nil
}
