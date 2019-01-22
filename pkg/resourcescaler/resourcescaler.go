package resourcescaler

import (
	"github.com/v3io/scaler/pkg"

	"github.com/nuclio/logger"
)

type EmptyResourceScaler struct{}

func New() scaler.ResourceScaler {
	return &EmptyResourceScaler{}
}

func (r *EmptyResourceScaler) SetScale(logger logger.Logger, namespace string, resource scaler.Resource, scale int) error {
	return nil
}

func (r *EmptyResourceScaler) GetResources(namespace string) ([]scaler.Resource, error) {
	return []scaler.Resource{}, nil
}

func (r *EmptyResourceScaler) GetConfig() (*scaler.ResourceScalerConfig, error) {
	return nil, nil
}
