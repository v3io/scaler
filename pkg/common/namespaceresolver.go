package common

import (
	"io/ioutil"
	"os"
)

func GetNamespace(namespaceArgument string) string {

	// if the namespace was passed in the arguments, use that
	if namespaceArgument != "" {
		return namespaceArgument
	}

	// if the namespace exists in env, use that
	if namespaceEnv := os.Getenv("SCALER_NAMESPACE"); namespaceEnv != "" {
		return namespaceEnv
	}

	// get namespace from within the pod. if found, return that
	if namespacePod, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return string(namespacePod)
	}

	return "default"
}
