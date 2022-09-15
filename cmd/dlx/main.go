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

package main

import (
	"flag"
	"os"

	"github.com/v3io/scaler/cmd/dlx/app"
	"github.com/v3io/scaler/pkg/common"

	"github.com/nuclio/errors"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig-path", os.Getenv("KUBECONFIG"), "Path of kubeconfig file")
	namespace := flag.String("namespace", "", "Kubernetes namespace")
	targetNameHeader := flag.String("target-name-header", "", "Name of the header that holds information on target name")
	targetPathHeader := flag.String("target-path-header", "", "Name of the header that holds information on target path")
	targetPort := flag.Int("target-port", 0, "Name of the header that holds information on target port")
	listenAddress := flag.String("listen-address", ":8090", "Address to listen upon for http proxy")
	resourceReadinessTimeout := flag.String("resource-readiness-timeout", "5m", "maximum wait time for the resource to be ready")
	multiTargetStrategy := flag.String("multi-target-strategy", "random", "Strategy for selecting to which target to send the request")
	flag.Parse()

	*namespace = common.GetNamespace(*namespace)

	if err := app.Run(*kubeconfigPath,
		*namespace,
		*targetNameHeader,
		*targetPathHeader,
		*targetPort,
		*listenAddress,
		*resourceReadinessTimeout,
		*multiTargetStrategy); err != nil {
		errors.PrintErrorStack(os.Stderr, err, 5)

		os.Exit(1)
	}
}
