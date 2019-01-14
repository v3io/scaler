package autoscaler

import (
	"testing"
	"time"

	"github.com/v3io/scaler/pkg"

	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type autoScalerTest struct {
	mock.Mock
	suite.Suite
	logger     logger.Logger
	autoscaler *Autoscaler
	ch         chan metricEntry
}

func (suite *autoScalerTest) SetScale(namespace string, resource scaler.Resource, scale int) error {
	suite.Called(namespace, resource)
	return nil
}

func (suite *autoScalerTest) GetResources() ([]scaler.Resource, error) {
	return []scaler.Resource{}, nil
}

func (suite *autoScalerTest) GetConfig() (*scaler.ResourceScalerConfig, error) {
	return nil, nil
}

func (suite *autoScalerTest) SetupTest() {
	var err error
	suite.ch = make(chan metricEntry)
	suite.autoscaler = &Autoscaler{
		logger:         suite.logger,
		metricsChannel: suite.ch,
		metricsMap:     make(resourceMetricTypeMap),
		resourceScaler: suite,
		metricName:     "fakeSource",
		threshold:      0,
	}
	suite.Require().NoError(err)
	suite.On("SetScale", mock.Anything, mock.Anything).Return()
	suite.Calls = []mock.Call{}
	suite.autoscaler.windowSize = time.Duration(1 * time.Minute)
}

func (suite *autoScalerTest) SetupSuite() {
	suite.logger, _ = nucliozap.NewNuclioZapTest("test")
}

func (suite *autoScalerTest) TestScaleToZero() {
	t, _ := time.ParseDuration("2m")

	suite.autoscaler.addMetricEntry("f", "fakeSource", metricEntry{
		timestamp:    time.Now().Add(-t),
		value:        0,
		resourceName: "f",
		metricName:   "fakeSource",
	})

	suite.autoscaler.checkResourceToScale(time.Now(), []scaler.Resource{"f"})

	suite.AssertNumberOfCalls(suite.T(), "SetScale", 1)
}

func (suite *autoScalerTest) TestNotScale() {
	t, _ := time.ParseDuration("5m")
	suite.autoscaler.windowSize = t

	for _, duration := range []string{"4m", "200s", "3m", "2m", "100s"} {
		suite.addEntry("f", duration, 0)
	}

	suite.autoscaler.checkResourceToScale(time.Now(), []scaler.Resource{"f"})
	suite.AssertNumberOfCalls(suite.T(), "SetScale", 0)

	for _, duration := range []string{"50s", "40s", "30s", "20s", "10s"} {
		suite.addEntry("f", duration, 0)
	}
	suite.addEntry("f", "5s", 9)

	suite.autoscaler.checkResourceToScale(time.Now(), []scaler.Resource{"f"})
	suite.AssertNumberOfCalls(suite.T(), "SetScale", 0)
}

func (suite *autoScalerTest) TestScaleToZeroWithNoEvents() {
	suite.autoscaler.checkResourceToScale(time.Now(), []scaler.Resource{"f"})
	suite.AssertNumberOfCalls(suite.T(), "SetScale", 0)
}

func (suite *autoScalerTest) TestScaleToZeroMultipleResources() {
	t1, _ := time.ParseDuration("2m")
	t2, _ := time.ParseDuration("30s")

	suite.autoscaler.addMetricEntry("foo", "fakeSource", metricEntry{
		timestamp:    time.Now().Add(-t1),
		value:        0,
		resourceName: "foo",
		metricName:   "fakeSource",
	})

	suite.autoscaler.addMetricEntry("bar", "fakeSource", metricEntry{
		timestamp:    time.Now().Add(-t2),
		value:        0,
		resourceName: "bar",
		metricName:   "fakeSource",
	})

	suite.autoscaler.checkResourceToScale(time.Now(), []scaler.Resource{"foo", "bar"})

	suite.AssertNumberOfCalls(suite.T(), "SetScale", 1)
}

func (suite *autoScalerTest) addEntry(key string, duration string, value int64) {
	t, _ := time.ParseDuration(duration)
	suite.autoscaler.addMetricEntry(scaler.Resource(key), "fakeSource", metricEntry{
		timestamp:    time.Now().Add(-t),
		value:        value,
		resourceName: "bb",
		metricName:   "fakeSource",
	})
}

func TestAutoscale(t *testing.T) {
	suite.Run(t, new(autoScalerTest))
}
