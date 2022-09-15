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

package pluginloader

import (
	"path/filepath"
	"plugin"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
)

type resourceScalerNewFunc func(string, string) (scalertypes.ResourceScaler, error)

type PluginLoader struct{}

func New() (*PluginLoader, error) {
	return &PluginLoader{}, nil
}

func (p *PluginLoader) Load(kubeconfigPath string, namespace string) (scalertypes.ResourceScaler, error) {
	var funcNew resourceScalerNewFunc
	var ok bool

	plugins, err := filepath.Glob("plugins/*.so")
	if err != nil || len(plugins) == 0 {
		return nil, errors.New("No plugins found")
	}

	// return the first found
	for _, filename := range plugins {
		p, err := plugin.Open(filename)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to open plugin")
		}

		symbol, err := p.Lookup("New")
		if err != nil {
			return nil, errors.Wrap(err, "Failed to find New symbol")
		}

		// don't use resourceScalerNewFunc as the cast type cause of some bug :(
		funcNew, ok = symbol.(func(string, string) (scalertypes.ResourceScaler, error))
		if !ok {
			return nil, errors.New("Failed to cast New function of plugin")
		}
	}

	return funcNew(kubeconfigPath, namespace)
}
