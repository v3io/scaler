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

type IngressHostCacheReader interface {
	// Get retrieves all target names for the given host and path
	Get(host string, path string) ([]string, error)
}

type IngressHostCache interface {
	IngressHostCacheReader

	// Set adds a new item to the cache for the given host, path and targets. Will overwrite existing values if any
	Set(host string, path string, targets []string) error

	// Delete removes the specified targets from the cache for the given host and path. Will do nothing if host, path or targets do not exist
	Delete(host string, path string, targets []string) error
}

type IngressHostsTree interface {
	// Set sets the targets for a given path. Will overwrite existing values if the path already exists
	Set(path string, targets []string) error

	// Delete removes the targets from the given path and deletes the deepest suffix used only by these targets; does nothing if the path or targets don't exist.
	Delete(path string, targets []string) error

	// Get retrieves the best matching targets for a given path based on longest prefix match
	Get(path string) (Target, error)

	// IsEmpty checks if the tree is empty
	IsEmpty() bool
}

// Target defines the trie.PathTrie value
type Target interface {
	// Equal returns true if the otherTarget is equal to the current target
	Equal(otherTarget Target) bool

	// ToSliceString returns a slice of targets
	ToSliceString() []string
}
