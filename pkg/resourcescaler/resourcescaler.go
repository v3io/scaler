package main

import (
	"github.com/v3io/scaler-types"
)

type NopResourceScaler struct{}

func New(kubeconfigPath string, namespace string) (scaler_types.ResourceScaler, error) { // nolint: deadcode
	return &NopResourceScaler{}, nil
}

func (r *NopResourceScaler) SetScale(resources []scaler_types.Resource, scale int) error {
	return nil
}

func (r *NopResourceScaler) GetResources() ([]scaler_types.Resource, error) {
	return []scaler_types.Resource{}, nil
}

func (r *NopResourceScaler) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	return nil, nil
}

func (r *NopResourceScaler) ResolveServiceName(resource scaler_types.Resource) (string, error) {
	return resource.Name, nil
}
