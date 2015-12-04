package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws/session"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/ec2"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/Godeps/_workspace/src/speter.net/go/exp/math/dec/inf"
	"github.com/YakLabs/kube-cloudwatch-node-metrics/resource"
)

type (
	// Resources holds the configuration. It is also used for caching.
	Resources struct {
		AutoScaleGroup string             `json:"autoscale"`
		InstanceID     string             `json:"instanceID"`
		NodeName       string             `json:"node"`
		CPUCapacity    *resource.Quantity `json:"CPUCapacity"`
		DefaultCPU     *resource.Quantity `json:"defaultCPU"`
		Region         string             `json:"region"`
	}

	// Metadata is the Kubernetes Metadata: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_objectmeta
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}

	// PodStatus is the Kubernetes PodStatus: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_podstatus
	PodStatus struct {
		Phase  string `json:"phase"`
		HostIP string `json:"hostIP"`
		PodIP  string `json:"podIP"`
	}

	// PodSpec is the Kubernetes PodSpec: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_podspec
	PodSpec struct {
		NodeName   string       `json:"nodeName"`
		Containers []*Container `json:"containers"`
	}

	// Pod is the Kubernetes Pod: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_pod
	Pod struct {
		Kind     string    `json:"kind"`
		Metadata Metadata  `json:"metadata"`
		Status   PodStatus `json:"status"`
		Spec     PodSpec   `json:"spec"`
	}

	// PodList is a list of Kubernetes Pods: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_podlist
	PodList struct {
		Items []*Pod `json:"items"`
	}

	// Container is a Kubernetes Container: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_container
	Container struct {
		Name      string               `json:"name"`
		Resources ResourceRequirements `json:"resources"`
	}

	// ResourceRequirements is a Kubernetes ResourceRequirement: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_resourcerequirements
	ResourceRequirements struct {
		Requests ContainerResources `json:"requests"`
		Limits   ContainerResources `json:"limits"`
	}

	// ContainerResources is a Kubernetes resource request or limit
	ContainerResources struct {
		CPU string `json:"cpu"`
	}

	// Node is a Kubernetes Node: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_node
	Node struct {
		Kind     string     `json:"kind"`
		Metadata Metadata   `json:"metadata"`
		Status   NodeStatus `json:"status"`
	}

	// NodeStatus is the Kubernetes NodeStatus: http://kubernetes.io/v1.1/docs/api-reference/v1/definitions.html#_v1_nodestatus
	NodeStatus struct {
		Capacity NodeCapacity `json:"capacity"`
	}

	//NodeCapacity is the Kubernetes resource capacity (cpu ane memory)
	NodeCapacity struct {
		CPU string `json:"CPU"`
	}
)

// TODO: support any auth?
func main() {
	var cacheFile, nodeName, instanceID, CPUCapacity, defaultCapacity, autoScaleGroup, kubeURL, region string
	var wait int
	flag.StringVar(&cacheFile, "cache", "/tmp/kube-resource-metrics.json", "cache file. set to \"\" to disable")
	flag.StringVar(&nodeName, "node", "", "node name. default it to autodetect")
	flag.StringVar(&instanceID, "instance", "", "AWS instance ID. default it to autodetect")
	flag.StringVar(&autoScaleGroup, "autoscale", "", "AWS autoscale group. default it to autodetect")
	flag.StringVar(&region, "region", "", "AWS region. default it to autodetect")
	flag.StringVar(&CPUCapacity, "CPU", "", "CPU cpacity of node. Default is to get from Kubernetes API")
	flag.StringVar(&defaultCapacity, "defaultmem", "250m", "default CPU requested if a container does not specify")
	flag.StringVar(&kubeURL, "kubeurl", "http://127.0.0.1:8001", "Kubernetes API. must not require auth. use kubectl proxy if auth is needed")
	flag.IntVar(&wait, "wait", 60, "how often to run. set to 0 for one time")
	flag.Parse()

	r := Resources{}

	// open and parse cache file if it exists
	if cacheFile != "" {
		_, err := os.Stat(cacheFile)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Fatal(err)
			}
		} else {
			data, err := ioutil.ReadFile(cacheFile)
			if err != nil {
				log.Fatal(err)
			}
			// an error here means an invalid cache file. Should be fine to continue.
			_ = json.Unmarshal(data, &r)
		}
	}

	// process any overrides
	if nodeName != "" {
		r.NodeName = nodeName
	}

	if instanceID != "" {
		r.InstanceID = instanceID
	}

	if autoScaleGroup != "" {
		r.AutoScaleGroup = autoScaleGroup
	}

	if region != "" {
		r.Region = region
	}

	if CPUCapacity != "" {
		q, err := resource.ParseQuantity(CPUCapacity)
		if err != nil {
			log.Fatal(err)
		}
		r.CPUCapacity = q
	}

	if defaultCapacity != "" {
		q, err := resource.ParseQuantity(defaultCapacity)
		if err != nil {
			log.Fatal(err)
		}
		r.DefaultCPU = q
	}

	if r.NodeName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("unable to get hostname: %s", err)
		}
		r.NodeName = hostname
	}

	if r.Region == "" {
		reg, err := getRegion()
		if err != nil {
			log.Fatalf("unable to get region: %s", err)
		}
		r.Region = reg
	}

	if r.InstanceID == "" {
		id, err := getInstanceID()
		if err != nil {
			log.Fatalf("unable to get instanceID: %s", err)
		}
		r.InstanceID = id
	}

	if r.AutoScaleGroup == "" {
		asg, err := getASG(r.InstanceID, r.Region)
		if err != nil {
			log.Fatalf("unable to get autoscale group: %s", err)
		}
		r.AutoScaleGroup = asg
	}

	if r.CPUCapacity == nil {
		cpu, err := getCPUCapacity(kubeURL, r.NodeName)
		if err != nil {
			log.Fatalf("unable to get CPU capacity: %s", err)
		}
		r.CPUCapacity = cpu
	}

	if cacheFile != "" {
		data, err := json.Marshal(r)
		if err != nil {
			log.Fatalf("unable to marshal cache data: %s", err)
		}
		err = ioutil.WriteFile(cacheFile, data, 0777)
		if err != nil {
			log.Fatalf("unable to write cache file to %s: %s", cacheFile, err)
		}
	}

	for {
		CPU, err := getRequestedCPU(kubeURL, r.NodeName, r.DefaultCPU)
		if err != nil {
			log.Printf("unable to determine CPU requests for %s - %s", r.NodeName, err)
		} else {
			err := postCloudWatchMetric(r.AutoScaleGroup, r.InstanceID, r.Region, CPU, r.CPUCapacity)
			if err != nil {
				log.Printf("unable to post metric: %s", err)
			}
		}
		if wait == 0 {
			break
		}
		time.Sleep(time.Duration(wait) * time.Second)
	}
}

func getCPUCapacity(kubeURL, nodeName string) (*resource.Quantity, error) {
	resp, err := http.Get(kubeURL + "/api/v1/nodes/" + nodeName)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var node Node

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &node); err != nil {
		return nil, err
	}
	q, err := resource.ParseQuantity(node.Status.Capacity.CPU)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func getRequestedCPU(kubeURL, nodeName string, defaultCPU *resource.Quantity) (*resource.Quantity, error) {
	resp, err := http.Get(kubeURL + "/api/v1/pods?fieldSelector=spec.nodeName=" + nodeName)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var podList PodList
	if err := json.Unmarshal(body, &podList); err != nil {
		return nil, err
	}

	cpu := resource.NewQuantity(0, "")

	// this counts pods in all states: pending, terminating, error, etc.
	// better to over report than under
	// request may be low and limits high - should we split the difference?
	for _, pod := range podList.Items {
		for _, c := range pod.Spec.Containers {
			var amt *resource.Quantity
			if c.Resources.Requests.CPU != "" {
				q, err := resource.ParseQuantity(c.Resources.Requests.CPU)
				if err != nil {
					log.Printf("unable to determine resource request for %s:%s - %s", pod.Metadata.Name, c.Name, err)
				} else {
					amt = q
				}
			}
			if amt == nil && c.Resources.Limits.CPU != "" {
				q, err := resource.ParseQuantity(c.Resources.Limits.CPU)
				if err != nil {
					log.Printf("unable to determine resource limits for %s:%s - %s", pod.Metadata.Name, c.Name, err)
				} else {
					amt = q
				}
			}

			if amt == nil {
				amt = defaultCPU
			}

			if err := cpu.Add(*amt); err != nil {
				log.Printf("unable to add CPU  for %s:%s - %s", pod.Metadata.Name, c.Name, err)
			}
		}
	}

	return cpu, nil
}

func getRegion() (string, error) {
	c := ec2metadata.New(session.New())
	return c.Region()
}

func getInstanceID() (string, error) {
	c := ec2metadata.New(session.New())
	return c.GetMetadata("instance-id")
}

func getASG(id, region string) (string, error) {
	svc := ec2.New(session.New(), &aws.Config{Region: aws.String(region)})
	params := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(id),
				},
			},
		},
	}
	resp, err := svc.DescribeTags(params)

	if err != nil {
		return "", err
	}

	for _, t := range resp.Tags {
		if t.Key != nil && *t.Key == "aws:autoscaling:groupName" {
			// just to be sure...
			if t.ResourceId != nil && *t.ResourceId == id {
				if t.Value != nil {
					return *t.Value, nil
				}
			}
		}
	}

	// unable to determine autoscale group, but no error occured
	return "", nil
}

func postCloudWatchMetric(asg, instancedID, region string, requestedCPU, CPUCapacity *resource.Quantity) error {
	svc := cloudwatch.New(session.New(), &aws.Config{Region: aws.String(region)})

	now := aws.Time(time.Now())

	requested := floatValue(requestedCPU)
	capacity := floatValue(CPUCapacity)

	input := &cloudwatch.PutMetricDataInput{
		MetricData: []*cloudwatch.MetricDatum{
			{
				MetricName: aws.String("KubernetesCPUPercent"),
				Dimensions: []*cloudwatch.Dimension{
					{
						Name:  aws.String("AutoScalingGroupName"),
						Value: aws.String(asg),
					},
				},
				Timestamp: now,
				Unit:      aws.String("Percent"),
				Value:     aws.Float64(float64(requested/capacity) * 100),
			},
		},
		Namespace: aws.String("Kubernetes"),
	}
	_, err := svc.PutMetricData(input)
	return err
}

func floatValue(q *resource.Quantity) float64 {
	if q.Amount == nil {
		return 0.0
	}
	tmp := &inf.Dec{}

	return float64(tmp.Round(tmp.Mul(q.Amount, inf.NewDec(1000, 0)), 3, inf.RoundUp).UnscaledBig().Int64() / 1000)
}
