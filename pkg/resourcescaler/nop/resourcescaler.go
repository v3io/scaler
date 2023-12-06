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

package nop

import (
	"context"

	"github.com/v3io/scaler/pkg/scalertypes"
)

type ResourceScaler struct{}

func New(_ string, _ string) (scalertypes.ResourceScaler, error) { // nolint: deadcode
	return &ResourceScaler{}, nil
}

func (r *ResourceScaler) SetScale(_ []scalertypes.Resource, _ int) error {
	return nil
}

func (r *ResourceScaler) SetScaleCtx(_ context.Context, _ []scalertypes.Resource, _ int) error {
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
