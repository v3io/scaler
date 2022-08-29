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
	"net/http"
	"sync"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type responseChannel chan ResourceStatusResult
type resourceSinksMap map[string]chan responseChannel

type ResourceStarter struct {
	logger                   logger.Logger
	namespace                string
	resourceSinksMap         resourceSinksMap
	resourceSinkMutex        sync.Mutex
	resourceReadinessTimeout time.Duration
	scaler                   scalertypes.ResourceScaler
}

type ResourceStatusResult struct {
	ResourceName string
	Status       int
	Error        error
}

func NewResourceStarter(parentLogger logger.Logger,
	scaler scalertypes.ResourceScaler,
	namespace string,
	resourceReadinessTimeout time.Duration) (*ResourceStarter, error) {
	fs := &ResourceStarter{
		logger:                   parentLogger.GetChild("resource-starter"),
		resourceSinksMap:         make(resourceSinksMap),
		namespace:                namespace,
		resourceReadinessTimeout: resourceReadinessTimeout,
		scaler:                   scaler,
	}
	return fs, nil
}

func (r *ResourceStarter) handleResourceStart(originalTarget string, handlerResponseChannel responseChannel) {
	resourceSinkChannel := r.getOrCreateResourceSink(originalTarget, handlerResponseChannel)
	resourceSinkChannel <- handlerResponseChannel
}

func (r *ResourceStarter) getOrCreateResourceSink(originalTarget string,
	handlerResponseChannel responseChannel) chan responseChannel {
	var resourceSinkChannel chan responseChannel
	r.resourceSinkMutex.Lock()
	defer r.resourceSinkMutex.Unlock()

	if _, found := r.resourceSinksMap[originalTarget]; found {
		resourceSinkChannel = r.resourceSinksMap[originalTarget]
	} else {

		// for the next requests coming in
		resourceSinkChannel = make(chan responseChannel)
		r.resourceSinksMap[originalTarget] = resourceSinkChannel
		r.logger.DebugWith("Created resource sink", "target", originalTarget)

		// start the resource and get ready to listen on resource sink channel
		go r.startResource(resourceSinkChannel, originalTarget)
	}

	return resourceSinkChannel
}

func (r *ResourceStarter) startResource(resourceSinkChannel chan responseChannel, target string) {
	var resultStatus ResourceStatusResult

	// simple for now
	resourceName := target

	r.logger.InfoWith("Starting resource", "resource", resourceName)
	resourceReadyChannel := make(chan error, 1)
	defer close(resourceReadyChannel)

	go r.waitResourceReadiness(scalertypes.Resource{
		Name: resourceName,

		// TODO: get a argument or it won't know which function on what namespace it should wake up
		Namespace: r.namespace,
	}, resourceReadyChannel)

	select {
	case <-time.After(r.resourceReadinessTimeout):
		r.logger.WarnWith("Timed out waiting for resource to be ready", "resource", resourceName)
		defer r.deleteResourceSink(resourceName)
		resultStatus = ResourceStatusResult{
			Error:        errors.New("Timed out waiting for resource to be ready"),
			Status:       http.StatusGatewayTimeout,
			ResourceName: resourceName,
		}
	case err := <-resourceReadyChannel:
		r.logger.InfoWith("Resource ready",
			"target", target,
			"err", errors.GetErrorStackString(err, 10))

		if err == nil {
			resultStatus = ResourceStatusResult{
				Status:       http.StatusOK,
				ResourceName: resourceName,
			}
		} else {
			resultStatus = ResourceStatusResult{
				Status:       http.StatusInternalServerError,
				ResourceName: resourceName,
				Error:        err,
			}
		}

	}

	// now handle all pending requests for a minute
	tc := time.After(1 * time.Minute)
	for {
		select {
		case channel := <-resourceSinkChannel:
			channel <- resultStatus
		case <-tc:
			r.logger.Debug("Releasing resource sink")
			r.deleteResourceSink(resourceName)
			return
		}
	}
}

func (r *ResourceStarter) waitResourceReadiness(resource scalertypes.Resource, resourceReadyChannel chan error) {
	err := r.scaler.SetScale([]scalertypes.Resource{resource}, 1)
	resourceReadyChannel <- err
}

func (r *ResourceStarter) deleteResourceSink(resourceName string) {
	r.resourceSinkMutex.Lock()
	delete(r.resourceSinksMap, resourceName)
	r.resourceSinkMutex.Unlock()
}
