package dlx

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	scaler_types "github.com/v3io/scaler-types"
)

type MockResourceScaler struct {
	mock.Mock
}

func (m *MockResourceScaler) SetScale(resourceName []scaler_types.Resource, scale int) error {
	args := m.Called(resourceName)
	return args.Error(0)
}

func (m *MockResourceScaler) SetScaleCtx(ctx context.Context, resourceName []scaler_types.Resource, scale int) error {
	args := m.Called(ctx, resourceName, scale)
	return args.Error(0)
}

func (m *MockResourceScaler) GetResources() ([]scaler_types.Resource, error) {
	args := m.Called()
	return args.Get(0).([]scaler_types.Resource), args.Error(1)
}

func (m *MockResourceScaler) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	args := m.Called()
	return args.Get(0).(*scaler_types.ResourceScalerConfig), args.Error(1)
}

func (m *MockResourceScaler) ResolveServiceName(resource scaler_types.Resource) (string, error) {
	args := m.Called(resource)
	return args.String(0), args.Error(1)
}

type resourceStarterTest struct {
	suite.Suite
	logger          logger.Logger
	functionStarter *ResourceStarter
	mocker          *MockResourceScaler
}

func (suite *resourceStarterTest) SetupTest() {
	suite.mocker = &MockResourceScaler{}
	suite.functionStarter = &ResourceStarter{
		logger:                   suite.logger,
		resourceSinksMap:         sync.Map{},
		namespace:                "default",
		resourceReadinessTimeout: 1 * time.Second,
		scaler:                   suite.mocker,
	}
}

func (suite *resourceStarterTest) SetupSuite() {
	suite.logger, _ = nucliozap.NewNuclioZapTest("test")
}

func (suite *resourceStarterTest) TestDlxMultipleRequests() {
	wg := sync.WaitGroup{}
	suite.mocker.
		On("SetScaleCtx", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(testIndex int) {
			defer wg.Done()
			ch := make(responseChannel)
			suite.functionStarter.handleResourceStart(fmt.Sprintf("test%d", testIndex), ch)
			r := <-ch
			suite.logger.DebugWith("Got response", "r", r)
			suite.Require().Equal(http.StatusOK, r.Status)
		}(i)
	}
	wg.Wait()
}

func (suite *resourceStarterTest) TestDlxMultipleRequestsSameTarget() {
	wg := sync.WaitGroup{}
	suite.mocker.
		On("SetScaleCtx", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

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
	suite.Require().True(suite.mocker.AssertNumberOfCalls(suite.T(), "SetScaleCtx", 1))
}

func TestResourceStarter(t *testing.T) {
	suite.Run(t, new(resourceStarterTest))
}
