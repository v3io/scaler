package dlx

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/v3io/scaler-types"
)

type resourceStarterTest struct {
	suite.Suite
	logger          logger.Logger
	functionStarter *ResourceStarter
	mocker          *mocker
}

type mocker struct {
	mock.Mock
	scaler_types.ResourceScaler
}

func (m *mocker) SetScale(resourceName []scaler_types.Resource, scale int) error {
	m.Called(resourceName)
	return nil
}

func (m *mocker) GetResources() ([]scaler_types.Resource, error) {
	return []scaler_types.Resource{}, nil
}

func (m *mocker) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	return nil, nil
}

func (suite *resourceStarterTest) SetupTest() {
	suite.mocker = &mocker{}
	suite.functionStarter = &ResourceStarter{
		logger:                   suite.logger,
		resourceSinksMap:         make(resourceSinksMap),
		namespace:                "default",
		resourceReadinessTimeout: time.Duration(1 * time.Second),
		scaler:                   suite.mocker,
	}
}

func (suite *resourceStarterTest) SetupSuite() {
	suite.logger, _ = nucliozap.NewNuclioZapTest("test")
}

func (suite *resourceStarterTest) TestDlxMultipleRequests() {
	wg := sync.WaitGroup{}
	suite.mocker.On("SetScale", mock.Anything).Return()

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			ch := make(responseChannel)
			suite.functionStarter.handleResourceStart(fmt.Sprintf("test%d", i), ch)
			r := <-ch
			suite.logger.DebugWith("Got response", "r", r)
			wg.Done()
			suite.Require().Equal(http.StatusOK, r.Status)
		}()
	}
	wg.Wait()
}

func (suite *resourceStarterTest) TestDlxMultipleRequestsSameTarget() {
	wg := sync.WaitGroup{}
	suite.mocker.On("SetScale", mock.Anything).Return()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			ch := make(responseChannel)
			suite.functionStarter.handleResourceStart("test", ch)
			r := <-ch
			suite.logger.DebugWith("Got response", "r", r)
			wg.Done()
			suite.Require().Equal(http.StatusOK, r.Status)
		}()
	}

	wg.Wait()
	suite.Require().True(suite.mocker.AssertNumberOfCalls(suite.T(), "SetScale", 1))
}

func TestResourceStarter(t *testing.T) {
	suite.Run(t, new(resourceStarterTest))
}
