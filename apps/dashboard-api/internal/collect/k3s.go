package collect

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/magmc/server-projects/dashboard-api/internal/config"
	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

// K3sCollector reads node/pod state and usage from the cluster.
type K3sCollector struct {
	clientset *kubernetes.Clientset
	metrics   *metricsv.Clientset
}

func NewK3sCollector(cfg config.Config) (*K3sCollector, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	if err != nil {
		return nil, err
	}
	// The kubeconfig points at 127.0.0.1:6443 (= this container on a bridge net).
	// Override to reach the host, and skip TLS verify since the API cert SAN
	// won't list the host-gateway name (trusted LAN, we already hold the creds).
	if cfg.K3sServer != "" {
		restCfg.Host = cfg.K3sServer
		restCfg.TLSClientConfig.Insecure = true
		restCfg.TLSClientConfig.CAData = nil
		restCfg.TLSClientConfig.CAFile = ""
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	metrics, err := metricsv.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	return &K3sCollector{clientset: clientset, metrics: metrics}, nil
}

func (k *K3sCollector) Collect(ctx context.Context) (model.K3sData, error) {
	nodes, err := k.collectNodes(ctx)
	if err != nil {
		return model.K3sData{}, err
	}
	pods, err := k.collectPods(ctx)
	if err != nil {
		return model.K3sData{}, err
	}
	return model.K3sData{Nodes: nodes, Pods: pods}, nil
}

func (k *K3sCollector) collectNodes(ctx context.Context) ([]model.K3sNode, error) {
	list, err := k.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Best-effort node usage from metrics-server (keyed by node name).
	usageCPU := map[string]int64{}
	usageMem := map[string]uint64{}
	if nm, err := k.metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{}); err == nil {
		for _, m := range nm.Items {
			usageCPU[m.Name] = m.Usage.Cpu().MilliValue()
			usageMem[m.Name] = uint64(m.Usage.Memory().Value())
		}
	}

	out := make([]model.K3sNode, 0, len(list.Items))
	for _, n := range list.Items {
		ready := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady {
				ready = c.Status == corev1.ConditionTrue
			}
		}
		node := model.K3sNode{Name: n.Name, Ready: ready}
		if cap := n.Status.Capacity.Cpu(); cap != nil {
			v := cap.MilliValue()
			node.CPUCapacityMilli = &v
		}
		if cap := n.Status.Capacity.Memory(); cap != nil {
			v := uint64(cap.Value())
			node.MemCapacityBytes = &v
		}
		if v, ok := usageCPU[n.Name]; ok {
			node.CPUMilli = &v
		}
		if v, ok := usageMem[n.Name]; ok {
			node.MemUsedBytes = &v
		}
		out = append(out, node)
	}
	return out, nil
}

func (k *K3sCollector) collectPods(ctx context.Context) ([]model.K3sPod, error) {
	list, err := k.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Best-effort pod usage from metrics-server (keyed by ns/name; summed over containers).
	usageCPU := map[string]int64{}
	usageMem := map[string]uint64{}
	if pm, err := k.metrics.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{}); err == nil {
		for _, m := range pm.Items {
			key := m.Namespace + "/" + m.Name
			var cpu int64
			var mem uint64
			for _, cont := range m.Containers {
				cpu += cont.Usage.Cpu().MilliValue()
				mem += uint64(cont.Usage.Memory().Value())
			}
			usageCPU[key] = cpu
			usageMem[key] = mem
		}
	}

	out := make([]model.K3sPod, 0, len(list.Items))
	for _, p := range list.Items {
		var restarts int32
		ready, total := 0, len(p.Status.ContainerStatuses)
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
			if cs.Ready {
				ready++
			}
		}
		pod := model.K3sPod{
			Namespace: p.Namespace,
			Name:      p.Name,
			Phase:     string(p.Status.Phase),
			Ready:     itoa(ready) + "/" + itoa(total),
			Restarts:  restarts,
		}
		key := p.Namespace + "/" + p.Name
		if v, ok := usageCPU[key]; ok {
			pod.CPUMilli = &v
		}
		if v, ok := usageMem[key]; ok {
			pod.MemUsedBytes = &v
		}
		out = append(out, pod)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
