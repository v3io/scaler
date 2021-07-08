package dlx

import (
	"context"
	"net/http"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type DLX struct {
	logger  logger.Logger
	handler Handler
	server  *http.Server
}

func NewDLX(parentLogger logger.Logger,
	resourceScaler scalertypes.ResourceScaler,
	options scalertypes.DLXOptions) (*DLX, error) {
	childLogger := parentLogger.GetChild("dlx")
	childLogger.InfoWith("Creating DLX", "options", options)
	resourceStarter, err := NewResourceStarter(childLogger,
		resourceScaler,
		options.Namespace,
		options.ResourceReadinessTimeout.Duration)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create function starter")
	}

	handler, err := NewHandler(childLogger,
		resourceStarter,
		resourceScaler,
		options.TargetNameHeader,
		options.TargetPathHeader,
		options.TargetPort)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create handler")
	}

	return &DLX{
		logger:  childLogger,
		handler: handler,
		server: &http.Server{
			Addr: options.ListenAddress,
		},
	}, nil
}

func (d *DLX) Start() error {
	d.logger.DebugWith("Starting", "server", d.server.Addr)
	http.HandleFunc("/", d.handler.HandleFunc)
	go d.server.ListenAndServe() // nolint: errcheck
	return nil
}

func (d *DLX) Stop(context context.Context) error {
	d.logger.DebugWith("Stopping", "server", d.server.Addr)
	return d.server.Shutdown(context)
}
