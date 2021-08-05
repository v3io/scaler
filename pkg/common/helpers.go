package common

import (
	"math/rand"
	"time"
)

var SeededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func UniquifyStringSlice(stringList []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range stringList {
		if _, exists := keys[entry]; !exists {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
