package nop

import (
	"github.com/v3io/scaler/pkg/scalertypes"
)

type ResourceScaler struct{}

func New(kubeconfigPath string, namespace string) (scalertypes.ResourceScaler, error) { // nolint: deadcode
	return &ResourceScaler{}, nil
}

func (r *ResourceScaler) SetScale(resources []scalertypes.Resource, scale int) error {
	return nil
}

func (r *ResourceScaler) GetResources() ([]scalertypes.Resource, error) {
	return []scalertypes.Resource{}, nil
}

func (r *ResourceScaler) GetConfig() (*scalertypes.ResourceScalerConfig, error) {
	return nil, nil
}

func (r *ResourceScaler) ResolveServiceName(resource scalertypes.Resource) (string, error) {
	return resource.Name, nil
}
