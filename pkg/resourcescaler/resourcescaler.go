package resourcescaler

import (
	"github.com/v3io/scaler/pkg"

	"github.com/nuclio/logger"
)

type ResourceScaler struct{}

func New() *ResourceScaler {
	return &ResourceScaler{}
}

func (r *ResourceScaler) SetScale(logger.Logger, string, scaler.Resource, int) error {
	return nil
}

func (r *ResourceScaler) GetResources() ([]scaler.Resource, error) {
	return []scaler.Resource{}, nil
}

func (r *ResourceScaler) GetConfig() (*scaler.ResourceScalerConfig, error) {
	return nil, nil
}
