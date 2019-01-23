package main

import (
	"github.com/v3io/scaler-types"
)

type NopResourceScaler struct{}

func New() (scaler_types.ResourceScaler, error) {
	return &NopResourceScaler{}, nil
}

func (r *NopResourceScaler) SetScale(namespace string, resource scaler_types.Resource, scale int) error {
	return nil
}

func (r *NopResourceScaler) GetResources(namespace string) ([]scaler_types.Resource, error) {
	return []scaler_types.Resource{}, nil
}

func (r *NopResourceScaler) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	return nil, nil
}
