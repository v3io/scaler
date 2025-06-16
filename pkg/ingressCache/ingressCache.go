package ingresscache

import (
	"sync"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type IngressHostsTree interface {
	SetFunctionName(path string, function string) error // will overwrite existing values if exists
	DeleteFunctionName(path string, function string) error
	GetFunctionName(path string) ([]string, error)
	IsEmpty() bool
}

type IngressHostCache interface {
	Set(host string, path string, function string) error // will overwrite existing values if exists
	Delete(host string, path string, function string) error
	Get(host string, path string) ([]string, error)
}

type IngressCache struct {
	syncMap *sync.Map
	logger  logger.Logger
}

func NewIngressCache(logger logger.Logger) *IngressCache {
	return &IngressCache{
		syncMap: &sync.Map{},
		logger:  logger,
	}
}

func (ic *IngressCache) Set(host, path, function string) error {
	urlTree, exists := ic.syncMap.Load(host)
	if !exists {
		urlTree = NewSafeTrie()
		ic.syncMap.Store(host, urlTree)
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		// remove the host from the cache when it's a new entry
		if !exists {
			ic.syncMap.Delete(host)
		}
		return errors.Errorf("cache set failed: invalid path tree value: got: %t", urlTree)
	}

	if err := ingressHostsTree.SetFunctionName(path, function); err != nil {
		// remove the host from the cache when it's a new entry
		if !exists {
			ic.syncMap.Delete(host)
		}
		return errors.Wrap(err, "cache set failed")
	}
	return nil
}

func (ic *IngressCache) Delete(host, path, function string) error {
	urlTree, exists := ic.syncMap.Load(host)
	if !exists {
		ic.logger.Debug("cache delete: host not found")
		return nil
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		return errors.Errorf("cache delete failed: invalid path tree value: got: %t", urlTree)
	}

	if err := ingressHostsTree.DeleteFunctionName(path, function); err != nil {
		return errors.Wrap(err, "cache delete failed")
	}

	if ingressHostsTree.IsEmpty() {
		// If the ingressHostsTree is empty after deletion, remove the host from the cache
		ic.logger.DebugWith("cache delete: host removed as it is empty",
			"host", host)
		ic.syncMap.Delete(host)
	}

	return nil
}

func (ic *IngressCache) Get(host, path string) ([]string, error) {
	urlTree, exists := ic.syncMap.Load(host)
	if !exists {
		return nil, errors.New("cache get failed: host does not exist")
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		return nil, errors.Errorf("cache get failed: invalid path tree value: got: %t", urlTree)
	}

	result, err := ingressHostsTree.GetFunctionName(path)
	if err != nil {
		return nil, errors.Wrap(err, "cache get failed")
	}

	return result, nil
}
