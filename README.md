# kube-cloudwatch-node-metrics
Emit AWS Cloudwatch metrics for Kubernetes node CPU usage for Autoscaling

`kube-cloudwatch-node-metrics` is a simple program that emits the
percentage of CPU "reserved" on a Kubernetes node.  It defines
"reserved" CPU as the sum of all the
[compute resources](http://kubernetes.io/v1.1/docs/user-guide/compute-resources.html)
of all the containers on a node.  It uses `requests` if set. It will
use `limits` if `requests` is not set, and a default if neither are
set.

`kube-cloudwatch-node-metrics` emits a single metric
`KubernetesCPUPercent` in the `Kubernetes` Cloudwatch namespace every
60 seconds. This can be used for alerting and/or for configuring AWS autoscaling.

`kube-cloudwatch-node-metrics` should be ran on each Kubernetes node.
A [daemonset](http://kubernetes.io/v1.1/docs/admin/daemons.html) is
the preferred method. An example [manifest](./daemonset.json) is included.

## Building and Using

This assumes you have enabled
[daemonsets](http://kubernetes.io/v1.1/docs/admin/daemons.html) in
your Kubernetes cluster.  The nodes also need permissions to describe
instances, describe tags and push
Cloudwatch metrics. A policy can be used, such as:

```json
{
    "Version" : "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeTags",
                "cloudwatch:PutMetricData"
            ],
            "Resource": "*"
            }
       ]
}
```

The simple way is to use the build script:
```shell
export DOCKER_REPO=my.docker.registry/mynamespace
docker login ${DOCKER_REPO}
./build.sh
```

This will build the binary and the Docker image. If `DOCKER_REPO` is
set, it will also push to it.

Edit  `daemonset.json` with the correct image information, and then
create the daemonset using `kubectl create -f daemonset.json`.
## Design

This initial version is extremely simple.

*It does not handle API authentication. It assumes you will be running
 [kubectl proxy](http://kubernetes.io/v1.1/docs/user-guide/kubectl/kubectl_proxy.html)
 in the same pod.
* It does not use watches. It fetches the pods for the node on every
run.
* It curently only uses the Kubernetes API. It could use the "pods"
end point on the kubelet.
* It does not use the official Kubernetes Go API client.  The official
  one has many dependencies and can challenging to build. This only
  uses a few structs, so doing it "by hand" is more straightforward.

## TODO

## LICENSE

see [LICENSE](./LICENSE)

## Authors

Written by the Engineering Team at [Yik Yak](http://www.yikyakapp.com/)
