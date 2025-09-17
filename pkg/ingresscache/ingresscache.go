/*
Copyright 2025 Iguazio Systems Ltd.

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

package ingresscache

import (
	"sync"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type IngressCache struct {
	syncMap *sync.Map
	logger  logger.Logger
}

func NewIngressCache(logger logger.Logger) *IngressCache {
	return &IngressCache{
		syncMap: &sync.Map{},
		logger:  logger.GetChild("cache"),
	}
}

func (ic *IngressCache) Set(host, path string, targets []string) error {
	urlTree, exists := ic.syncMap.LoadOrStore(host, NewSafeTrie())

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		if !exists {
			ic.syncMap.Delete(host)
		}
		return errors.Errorf("cache set failed: invalid path tree value: got: %t", urlTree)
	}

	if err := ingressHostsTree.Set(path, targets); err != nil {
		if !exists {
			ic.syncMap.Delete(host)
		}

		return errors.Wrap(err, "failed to set targets in the ingress host tree")
	}

	return nil
}

func (ic *IngressCache) Delete(host, path string, targets []string) error {
	urlTree, exists := ic.syncMap.Load(host)
	if !exists {
		ic.logger.Debug("cache delete: host not found")
		return nil
	}

	ingressHostsTree, ok := urlTree.(IngressHostsTree)
	if !ok {
		return errors.Errorf("cache delete failed: invalid path tree value: got: %t", urlTree)
	}

	if err := ingressHostsTree.Delete(path, targets); err != nil {
		return errors.Wrap(err, "failed to delete targets from the ingress host tree")
	}

	if ingressHostsTree.IsEmpty() {
		// If the ingressHostsTree is empty after deletion, remove the host from the cache
		ic.syncMap.Delete(host)
		ic.logger.DebugWith("cache delete: host removed as it is empty",
			"host", host)
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

	result, err := ingressHostsTree.Get(path)
	if err != nil {
		// If the specific path lookup fails, retry with root ("/").
		// Needed because the trie can’t resolve prefixes when "/" is both delimiter and root path.
		result, err = ingressHostsTree.Get("/")
		if err != nil {
			return nil, errors.Wrap(err, "failed to get the targets from the ingress host tree")
		}
	}

	return result.ToSliceString(), nil
}
