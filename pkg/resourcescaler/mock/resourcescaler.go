/*
Copyright 2019 Iguazio Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License") with
an addition restriction as set forth herein. You may not use this
file except in compliance with the License. You may obtain a copy of
the License at http://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing
permissions and limitations under the License.

In addition, you may not use the software for any purposes that are
illegal under applicable law, and the grant of the foregoing license
under the Apache 2.0 license is conditioned upon your compliance with
such restriction.
*/
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
