package resourcescaler

import "github.com/v3io/scaler/pkg"

type resourceScaler struct {}


func New() *resourceScaler {
	return &resourceScaler{}
}

func (r *resourceScaler) SetScale(string, scaler.Resource, int) error {
	return nil
}

func (r *resourceScaler) GetResources() ([]scaler.Resource, error) {
	return []scaler.Resource{}, nil
}

func (r *resourceScaler) GetConfig() (*scaler.ResourceScalerConfig, error) {
	return nil, nil
}
