package ingressCache

import (
	"fmt"
	"sync"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type IngressHostsTree interface {
	SetPath(path string, function string) error // will overwrite existing values if exists
	DeletePath(path string, function string) error
	GetPath(path string) ([]string, error)
}

type IngressHostCache interface {
	Set(host string, path string, function string) error // will overwrite existing values if exists
	Delete(host string, path string, function string) error
	Get(host string, path string) ([]string, error)
}

type IngressCache struct {
	m      *sync.Map
	logger logger.Logger
}

func NewIngressCache(logger logger.Logger) *IngressCache {
	return &IngressCache{
		m:      &sync.Map{},
		logger: logger,
	}
}

func (c *IngressCache) Set(host, path, function string) error {
	urlTree, exists := c.m.Load(host)
	if !exists {
		urlTree = newPathTree()
		c.m.Store(host, urlTree)
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		// if the host was not exists before, we should delete it from the cache
		if !exists {
			c.m.Delete(host)
		}
		return errors.New(fmt.Sprintf("cache set failed: invalid path tree value: got: %t", urlTree))
	}

	if err := ingressHostsTree.SetPath(path, function); err != nil {
		// if the host was not exists before, we should delete it from the cache
		if !exists {
			c.m.Delete(host)
		}
		return errors.Wrap(err, "cache set failed")
	}
	return nil
}

func (c *IngressCache) Delete(host, path, function string) error {
	urlTree, exists := c.m.Load(host)
	if !exists {
		c.logger.Debug("cache delete: host not found")
		return nil
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		return errors.New(fmt.Sprintf("cache delete failed: invalid path tree value: got: %t", urlTree))
	}

	if err := ingressHostsTree.DeletePath(path, function); err != nil {
		return errors.Wrap(err, "cache delete failed")
	}

	//todo - need to think bout how to know if the host should be Deleted from teh syncMap
	// should extend the IngressHostsTree interface with IsEmpty method
	return nil
}

func (c *IngressCache) Get(host, path string) ([]string, error) {
	urlTree, exists := c.m.Load(host)
	if !exists {
		return nil, errors.New("cache get failed: host does not exist")
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		return nil, errors.New(fmt.Sprintf("cache get failed: invalid path tree value: got: %t", urlTree))
	}

	result, err := ingressHostsTree.GetPath(path)
	if err != nil {
		return nil, errors.Wrap(err, "cache get failed")
	}

	return result, nil
}
