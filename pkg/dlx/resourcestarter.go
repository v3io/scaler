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
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type responseChannel chan ResourceStatusResult

type ResourceStarter struct {
	logger                   logger.Logger
	namespace                string
	resourceSinksMap         sync.Map
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
		resourceSinksMap:         sync.Map{},
		namespace:                namespace,
		resourceReadinessTimeout: resourceReadinessTimeout,
		scaler:                   scaler,
	}
	return fs, nil
}

func (r *ResourceStarter) handleResourceStart(originalTarget string, handlerResponseChannel responseChannel) {
	r.getOrCreateResourceSink(originalTarget) <- handlerResponseChannel
}

func (r *ResourceStarter) getOrCreateResourceSink(originalTarget string) chan responseChannel {
	resourceSink, found := r.resourceSinksMap.LoadOrStore(originalTarget, make(chan responseChannel))
	resourceSinkChannel := resourceSink.(chan responseChannel)
	if !found {
		ctx := context.Background()
		r.logger.DebugWithCtx(ctx, "Starting resource sink", "target", originalTarget)

		// for the next requests coming in
		// start the resource and get ready to listen on resource sink channel
		go r.startResource(ctx, resourceSinkChannel, originalTarget)
	}

	return resourceSinkChannel
}

func (r *ResourceStarter) startResource(ctx context.Context, resourceSinkChannel chan responseChannel, target string) {
	var resultStatus ResourceStatusResult

	// simple for now
	resourceName := target

	r.logger.InfoWithCtx(ctx, "Starting resource", "resourceName", resourceName)

	resourceReadyChannel := make(chan error, 1)

	// since defer is LIFO, this will be called last as we want.
	// reason - we want to close the channel only after we cancel the waitResourceReadiness
	// to avoid closing a channel that is still being used
	defer close(resourceReadyChannel)

	waitResourceReadinessCtx, cancelFuncTimeout := context.WithTimeout(ctx, 15*time.Minute)
	defer cancelFuncTimeout()

	go r.waitResourceReadiness(waitResourceReadinessCtx,
		scalertypes.Resource{Name: resourceName,

			// TODO: get a argument or it won't know which function on what namespace it should wake up
			Namespace: r.namespace},
		resourceReadyChannel)

	select {
	case <-time.After(r.resourceReadinessTimeout):
		r.logger.WarnWithCtx(ctx,
			"Timed out waiting for resource to be ready",
			"resourceName", resourceName)
		defer r.deleteResourceSink(resourceName)
		resultStatus = ResourceStatusResult{
			Error:        errors.New("Timed out waiting for resource to be ready"),
			Status:       http.StatusGatewayTimeout,
			ResourceName: resourceName,
		}
	case err := <-resourceReadyChannel:

		logArgs := []interface{}{
			"resourceName", resourceName,
			"target", target,
		}
		if err != nil {
			logArgs = append(logArgs, "err", errors.GetErrorStackString(err, 10))
		}
		r.logger.InfoWithCtx(ctx,
			"Resource ready",
			logArgs...,
		)

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
			r.deleteResourceSink(resourceName)
			return
		}
	}
}

func (r *ResourceStarter) waitResourceReadiness(ctx context.Context,
	resource scalertypes.Resource,
	resourceReadyChannel chan error) {

	err := r.scaler.SetScaleCtx(ctx, []scalertypes.Resource{resource}, 1)

	// callee decided to cancel, the resourceReadyChannel is already closed,
	// so we can just return without sending anything
	if ctx.Err() != nil {
		r.logger.WarnWithCtx(ctx,
			"Wait resource readiness canceled",
			"resourceName", resource.Name,
			"err", ctx.Err())
		return
	}
	resourceReadyChannel <- err
}

func (r *ResourceStarter) deleteResourceSink(resourceName string) {
	r.resourceSinksMap.Delete(resourceName)
}
