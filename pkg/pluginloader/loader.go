package pluginloader

import (
	"path/filepath"
	"plugin"

	"github.com/nuclio/errors"
	"github.com/v3io/scaler-types"
)

type resourceScalerNewFunc func(string, string) (scaler_types.ResourceScaler, error)

type PluginLoader struct{}

func New() (*PluginLoader, error) {
	return &PluginLoader{}, nil
}

func (p *PluginLoader) Load(kubeconfigPath string, namespace string) (scaler_types.ResourceScaler, error) {
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

		funcNew, ok = symbol.(resourceScalerNewFunc)
		if !ok {
			return nil, errors.New("Failed to cast New function of plugin")
		}
	}

	return funcNew(kubeconfigPath, namespace)
}
