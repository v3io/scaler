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
package dlx

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	mockresourcescaler "github.com/v3io/scaler/pkg/resourcescaler/mock"

	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type resourceStarterTest struct {
	suite.Suite
	logger          logger.Logger
	functionStarter *ResourceStarter
	mocker          *mockresourcescaler.ResourceScaler
}

func (suite *resourceStarterTest) SetupTest() {
	suite.mocker = &mockresourcescaler.ResourceScaler{}
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
