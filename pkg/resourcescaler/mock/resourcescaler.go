package mock

import (
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/stretchr/testify/mock"
)

type ResourceScaler struct {
	mock.Mock
}

func New(kubeconfigPath string, namespace string) (scalertypes.ResourceScaler, error) { // nolint: deadcode
	return &ResourceScaler{}, nil
}

func (r *ResourceScaler) SetScale(resources []scalertypes.Resource, scale int) error {
	args := r.Called(resources, scale)
	return args.Error(0)
}

func (r *ResourceScaler) GetResources() ([]scalertypes.Resource, error) {
	args := r.Called()
	return args.Get(0).([]scalertypes.Resource), args.Error(1)
}

func (r *ResourceScaler) GetConfig() (*scalertypes.ResourceScalerConfig, error) {
	args := r.Called()
	return args.Get(0).(*scalertypes.ResourceScalerConfig), args.Error(1)
}

func (r *ResourceScaler) ResolveServiceName(resource scalertypes.Resource) (string, error) {
	args := r.Called(resource)
	return args.Get(0).(string), args.Error(1)
}
