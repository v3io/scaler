<a id="top"></a>
[![Build Status](https://github.com/v3io/scaler/workflows/CI/badge.svg)](https://github.com/v3io/scaler/actions)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

# Scale to Zero

Infrastructure to scale any resource to and from zero

## Design:
![Design scheme](https://www.lucidchart.com/publicSegments/view/b01741e1-cb15-438e-8bd7-e5865dcc2043/image.jpeg)

**Resource** - A generic name for the component which is being scaled by this mechanism.
Will mostly include pod/s (that will be scaled when needed) wrapped by one or many deployment/daemon set/replica set, 
and k8s service/s that can be used to route incoming requests.

**Resource-scaler** - An implementation of the `ResourceScaler` interface defined in 
[scaler-types](https://github.com/v3io/scaler-types). It is transplanted inside the autoscaler and dlx using
[Go plugins](https://appliedgo.net/plugins/). They use them to perform actions on the specific resource.<br>
For example, when the autoscaler decides it needs to scale some resource to zero, it executes the resource-scaler's
`SetScale` function which has the knowledge how to scale to zero its specific resource.

**The autoscaler** - Responsible for periodically checking whether some resources should be scaled to zero. This is 
performed by by querying the custom metrics API. Upon deciding a resource should be scaled to zero, it uses the internal 
resource-scaler module to scale the resource to zero.
The resource-scaler will first route all incoming traffic to the DLX, which in terms of K8s is done by changing a 
service selector, after that, it will scale the resource to zero.

**The DLX** - Responsible for receiving and buffering requests of scaled to zero resources, Upon receiving an incoming 
request it creates a buffer for the messages, and tells the resource-scaler to scale the service back from zero.
The resource-scaler will scale the resource back up and then route the traffic back to the service (by modifying the k8s 
service selector).

## Prerequisites

**Custom metrics API implementation:**

The Autoscaler makes decisions based on data queried from Kubernetes 
[custom metrics API.](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/instrumentation/custom-metrics-api.md)
There are several possible tools that implement it, we internally use
[Prometheus](https://prometheus.io/) with the [Prometheus-Adapter](https://github.com/DirectXMan12/k8s-prometheus-adapter)
but you can use which ever you want! You can find some recommended implementations 
[here](https://github.com/kubernetes/metrics/blob/master/IMPLEMENTATIONS.md#custom-metrics-api)

## Getting Started
The infrastructure is designed to be generic, flexible and extendable, so as to serve any resource we'd wish to scale 
to/from zero. All you have to do is implement the specific resource-scaler for your resource. The interface between your 
resource-scaler and the scale-to-zero infrastructure's components is defined in 
[scaler-types](https://github.com/v3io/scaler-types)

**Note:** Incompatibility between this scaler vendor dir and your resource-scale vendor dir may break things, 
therefore it's suggested to put your resource-scaler in its own repo

Examples:
* [Nuclio functions resource-scaler](https://github.com/nuclio/nuclio/blob/master/pkg/platform/kube/resourcescaler/resourcescaler.go)
* [Iguazio's app service resource-scaler](https://github.com/v3io/app-resource-scaler/blob/development/resourcescaler.go)  

## Installing
[Go plugins](https://appliedgo.net/plugins/) is the magic that glues the resource-scaler and this infrastructure 
components together.<br>
First you'll need to build the resource-scaler as a Go plugin, for example: <br>
```sh
GOOS=linux GOARCH=amd64 go build -buildmode=plugin -a -installsuffix cgo -ldflags="-s -w" -o ./plugin.so ./resourcescaler.go
```
The autoscaler/dlx looks for the plugin using this path (from the execution directory) `./plugins/*.so` so you should 
move the binary artifact of the build command (the `plugin.so` file) to the `plugins` directory
It is much easier to do everything using Dockerfiles, here are some great examples: 
* [Nuclio function Autoscaler dockerfile](https://github.com/nuclio/nuclio/blob/master/cmd/autoscaler/Dockerfile)  
* [Nuclio function DLX dockerfile](https://github.com/nuclio/nuclio/blob/master/cmd/dlx/Dockerfile)
* [Iguazio's app service Autoscaler dockerfile](https://github.com/v3io/app-resource-scaler/blob/development/autoscaler/Dockerfile)  
* [Iguazio's app service DLX dockerfile](https://github.com/v3io/app-resource-scaler/blob/development/dlx/Dockerfile)

You can install the components using the [scaler helm chart](https://github.com/v3io/helm-charts/tree/development/stable/scaler)<br>
`$ helm install --name my-release v3io-stable/scaler`


## Versioning

We use [SemVer](http://semver.org/) for versioning. For the versions available, see the 
[releases on this repository](https://github.com/v3io/scaler/releases).
