package ingressCache

import (
	"fmt"
	"github.com/dghubble/trie"
)

const (
	emptyPathError         = "path is empty"
	emptyFunctionNameError = "function name is empty"
	walkPathResultError    = "value is not a []string"
	functionNotExistsError = "function does not exist in path"
)

// newPathTree creates a new PathTree instance
func newPathTree() *pathTree {
	return &pathTree{*trie.NewPathTrie()}
}

type pathTree struct {
	trie.PathTrie
}

// SetPath sets a function for a given path. If the path does not exist, it creates it
func (p *pathTree) SetPath(path string, function string) error {
	if path == "" {
		return fmt.Errorf(emptyPathError)
	}

	if function == "" {
		return fmt.Errorf(emptyFunctionNameError)
	}

	// get the exact path value in order to avoid creating a new path if it does not exist
	pathValue := p.Get(path)
	if pathValue == nil {
		p.Put(path, []string{function})
		return nil
	}

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return fmt.Errorf("%s, got %T", walkPathResultError, pathValue)
	}

	if existsInSlice(pathFunctionNames, function) {
		return nil
	}

	pathValue = append(pathFunctionNames, function)
	p.Put(path, pathValue)

	return nil
}

// DeletePath removes a function from a path and also deletes the path if the function is the only one associated with that path
func (p *pathTree) DeletePath(path string, function string) error {
	pathValue := p.Get(path)

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return fmt.Errorf("%s, got %T", functionNotExistsError, pathValue)
	}

	// If the function is the only value, delete the path
	if len(pathFunctionNames) == 1 {
		if pathFunctionNames[0] == function {
			p.Delete(path)
			return nil
		}
		return fmt.Errorf("%s,  function:%s, path:%s", functionNotExistsError, function, path)
	}

	pathFunctionNames = excludeElemFromSlice(pathFunctionNames, function)
	p.Put(path, pathFunctionNames)
	return nil
}

// GetPath retrieve the closest prefix matching the path and returns the associated functions
func (p *pathTree) GetPath(path string) ([]string, error) {
	var walkPathResult interface{}
	err := p.WalkPath(path, func(path string, value interface{}) error {
		if value != nil {
			walkPathResult = value
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("no value found for path: %s", path)
	}

	output, ok := walkPathResult.([]string)
	if !ok {
		return nil, fmt.Errorf("%s, value: %v", walkPathResultError, walkPathResult)
	}

	return output, err
}

func existsInSlice(slice []string, elem string) bool {
	for _, v := range slice {
		if v == elem {
			return true
		}
	}
	return false
}

func excludeElemFromSlice(slice []string, elem string) []string {
	var output []string
	for _, v := range slice {
		if v == elem {
			continue
		}
		output = append(output, v)
	}
	return output
}
