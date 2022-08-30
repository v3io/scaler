package dlx

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
)

type responseChannel chan ResourceStatusResult

type ResourceStarter struct {
	logger                   logger.Logger
	namespace                string
	resourceSinksMap         sync.Map
	resourceReadinessTimeout time.Duration
	scaler                   scaler_types.ResourceScaler
}

type ResourceStatusResult struct {
	ResourceName string
	Status       int
	Error        error
}

func NewResourceStarter(parentLogger logger.Logger,
	scaler scaler_types.ResourceScaler,
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

	waitResourceReadinessCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	go r.waitResourceReadiness(waitResourceReadinessCtx,
		scaler_types.Resource{Name: resourceName},
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
	resource scaler_types.Resource,
	resourceReadyChannel chan error) {

	err := r.scaler.SetScaleCtx(ctx, []scaler_types.Resource{resource}, 1)
	resourceReadyChannel <- err
}

func (r *ResourceStarter) deleteResourceSink(resourceName string) {
	r.resourceSinksMap.Delete(resourceName)
}
