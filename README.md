# Scaler
[![Build Status](https://travis-ci.org/v3io/scaler.svg?branch=master)](https://travis-ci.org/v3io/scaler)

Building blocks to create a scaler (to and from zero instances) based on custom metrics.

Prerequisites:

https://github.com/prometheus/prometheus <br>
https://github.com/kubernetes-incubator/metrics-server <br>
https://github.com/DirectXMan12/k8s-prometheus-adapter <br>

Schema:
```

                   Your Pod
                   To scale
 ------------     ------------       ------------       ------------       ------------       ------------  
|            |   | /metrics   |     | Prometheus |     | Prometheus |     | Metrics    |     | Metrics    |                
| Service    |-> |            |<--- |            |<--- | Adapter    |<--- | Aggregator |---> | Server     |
|            |   |            |     |            |     |            |     |            |     |            |
 ------------     ------------       ------------       ------------       ------------       ------------   
      |                                                                         /\
      |                                                                     ------------ 
      | Resource                                                           |            |
      | Scaled to zero                                                     | Autoscaler |
      |                                                                    |            |
      |                                                                     ------------
      |                                                    
      |                     ------------                 
      |                    |            |
       ------------------> |    DLX     |
                           |            |         
                            ------------   
                                                         
                                                          
                                                         
```

### Autoscaler
Based on custom metric name, will call `ResourceScaler` interface function `Scale` with a `Resource` from a list of known `Resource`'s (`GetResources` function)
Config for service includes:
```
    Namespace     - kubernetes namespace of the resources
    ScaleWindow   - an allowed period for the resource to be inactive before calling the `Scale` function
    MetricName    - name of the metric to monitor for the resource
    Threshold     - A threshold for a metric to concider the resource being inactive
    ScaleInterval - an interval to call scale function which checks if a resource has passed the scale window, if not the scale window is being considered the longest duration of time where the metric for the resource is under the threshold
```
Scaler should change the resource service to point to DLX service and not to the scaled down pod (to be changed by `ResourceScaler` or some other external entity)

### DLX
A service that listens on http requests for a given namespace, when a request is received, it's being saved in-memory until `Scale` function returns, this is an indication that the resource is ready and any changes that were made by autoscaler were reverted (i.e. changing where service points to), that request is then being proxied to the new instance and the response from it returns to the user.

