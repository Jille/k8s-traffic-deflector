package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/Jille/convreq"
	"github.com/Jille/convreq/respond"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	port       = flag.Int("port", 8080, "The port to serve on")

	mtx      sync.Mutex
	ipHealth = map[string]bool{}
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	nw, err := kubeClient.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Fatalf("Error fetching node: %s", err.Error())
	}
	defer nw.Stop()

	http.Handle("/", convreq.Wrap(httpHandler))
	go func() {
		klog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
	}()

	for e := range nw.ResultChan() {
		switch e.Type {
		case watch.Added, watch.Modified, watch.Deleted:
			n, ok := e.Object.(*corev1.Node)
			if !ok {
				continue
			}
			drained := false
			for _, t := range n.Spec.Taints {
				if t.Effect == corev1.TaintEffectNoExecute || t.Effect == corev1.TaintEffectNoSchedule {
					drained = true
				}
			}
			mtx.Lock()
			for _, a := range n.Status.Addresses {
				ipHealth[a.Address] = e.Type != watch.Deleted && !drained
			}
			if a := n.ObjectMeta.Annotations["io.cilium.network.ipv4-cilium-host"]; a != "" {
				ipHealth[a] = e.Type != watch.Deleted && !drained
			}
			mtx.Unlock()
		case watch.Bookmark:
		case watch.Error:
			klog.Errorf("Error on watch stream: %s", e.Object)
		}
		klog.Infof("State map: %v", ipHealth)
	}
	klog.Fatal("Watcher loop died")
}

func httpHandler(r *http.Request) convreq.HttpResponse {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		h = r.RemoteAddr
	}
	mtx.Lock()
	healthy, found := ipHealth[h]
	mtx.Unlock()
	if !found {
		return respond.String("Fine (node " + h + " unknown)")
	}
	if !healthy {
		return respond.OverrideResponseCode(respond.String("Node has NoExecute"), http.StatusServiceUnavailable)
	}
	return respond.String("Healthy")
}
