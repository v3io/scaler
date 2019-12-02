# Scale to Zero

Infrastructure to scale any resource to and from zero

## Prerequisites

**Custom metrics API implementation:**

The Autoscaler takes decisions based on data queried from Kubernetes 
[custom metrics API.](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/instrumentation/custom-metrics-api.md)
There are several possible tools that implement it, we internally use
[Prometheus](https://prometheus.io/) with the [Prometheus-Adapter](https://github.com/DirectXMan12/k8s-prometheus-adapter)
but you can use which ever you want... you can find [here](https://github.com/kubernetes/metrics/blob/master/IMPLEMENTATIONS.md#custom-metrics-api) 
recommended implementations

## Design:
![Design scheme](https://www.lucidchart.com/publicSegments/view/cc8927a6-537f-4fe6-95e1-731503bc7996/image.jpeg)

**The autoscaler** - responsible to periodically check whether resources should be scale to zero, it does that by 
querying the custom metrics API. When it decide a resource should be scaled to zero it tells the resource scaler to 
scale the resource to zero.
The resource scaler will route the traffic to the DLX, which in terms of K8s is done by changing the svc selector, 
after that, it will scale the resource to zero.

**The DLX** - responsible for receiving and buffering requests of scaled to zero resources, when it gets a message it 
creates a buffer for the messages, and tells the resource scaler to scale the service back from zero.
The resource scaler will scale the resource back up and then route the traffic back to the service (by changing the svc 
selector).

## Getting Started
This infrastructure designed to be generic and extendable, it can scale any resource.
All you have to do is implement the resource scaler, the interface between it and this infrastructure's components is 
defined in [scaler-types](https://github.com/v3io/scaler-types)

**Note:** Incompatibility between the scaler repo vendor dir and the resource scale vendor dir may break things, 
therefore it's suggested to put the resource scaler in its own repo

Examples:
* [Nuclio functions resource scaler](https://github.com/nuclio/nuclio/blob/master/pkg/platform/kube/resourcescaler/resourcescaler.go)
* [Iguazio's app service resource scaler](https://github.com/v3io/app-resource-scaler/blob/development/resourcescaler.go)  

## Installing
Go plugin is the magic that glues the resource scaler and this infrastructure components together
First you'll need to build a Dockerfile that builds your resource scaler as a Go plugin, and transplant it in this 
repo released images

Here's some great examples:
* [Nuclio function Autoscaler dockerfile](https://github.com/nuclio/nuclio/blob/master/cmd/autoscaler/Dockerfile)  
* [Nuclio function DLX dockerfile](https://github.com/nuclio/nuclio/blob/master/cmd/dlx/Dockerfile)
* [Iguazio's app service Autoscaler dockerfile](https://github.com/v3io/app-resource-scaler/blob/development/autoscaler/Dockerfile)  
* [Iguazio's app service DLX dockerfile](https://github.com/v3io/app-resource-scaler/blob/development/dlx/Dockerfile)


## Versioning

We use [SemVer](http://semver.org/) for versioning. For the versions available, see the 
[releases on this repository](https://github.com/v3io/scaler/releases).
