package pluginloader

import (
	"path/filepath"
	"plugin"

	"github.com/nuclio/errors"
	"github.com/v3io/scaler-types"
)

type PluginLoader struct{}

func New() (*PluginLoader, error) {
	return &PluginLoader{}, nil
}

func (p *PluginLoader) Load() (scaler_types.ResourceScaler, error) {
	plugins, err := filepath.Glob("plugins/*.so")
	if err != nil {
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

		funcNew, ok := symbol.(func() (scaler_types.ResourceScaler, error))
		if !ok {
			return nil, errors.New("Failed to cast New function of plugin")
		}

		return funcNew()
	}

	return nil, errors.New("No plugins found")
}
