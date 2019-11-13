package dlx

import (
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

	r.logger.DebugWith("Starting resource", "resource", resourceName)
	resourceReadyChannel := make(chan error, 1)
	defer close(resourceReadyChannel)

	go r.waitResourceReadiness(scaler_types.Resource{Name: resourceName}, resourceReadyChannel)

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
		r.logger.DebugWith("Resource ready", "target", target, "err", errors.GetErrorStackString(err, 10))

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

func (r *ResourceStarter) waitResourceReadiness(resource scaler_types.Resource, resourceReadyChannel chan error) {
	err := r.scaler.SetScale(resource, 1)
	resourceReadyChannel <- err
}

func (r *ResourceStarter) deleteResourceSink(resourceName string) {
	r.resourceSinkMutex.Lock()
	delete(r.resourceSinksMap, resourceName)
	r.resourceSinkMutex.Unlock()
}
