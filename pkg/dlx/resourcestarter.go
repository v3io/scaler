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
type resourceSinksMap map[string]chan responseChannel

type ResourceStarter struct {
	logger                   logger.Logger
	namespace                string
	resourceSinksMap         resourceSinksMap
	resourceSinkMutex        sync.Mutex
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
		ctx := context.Background()

		// for the next requests coming in
		resourceSinkChannel = make(chan responseChannel)
		r.resourceSinksMap[originalTarget] = resourceSinkChannel
		r.logger.DebugWithCtx(ctx, "Created resource sink", "target", originalTarget)

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
			r.logger.DebugWithCtx(ctx, "Releasing resource sink")
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
	r.resourceSinkMutex.Lock()
	delete(r.resourceSinksMap, resourceName)
	r.resourceSinkMutex.Unlock()
}
