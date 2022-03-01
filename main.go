package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	namespace             = flag.String("ns", "default", "namespace")
	podName               = flag.String("pn", "", "pod name")
	containerName         = flag.String("cn", "", "container name")
	labels                = flag.String("l", "", "app=mysql,version=v1.1.2")
	waitRunningPodTimeout = flag.Duration("wp", time.Minute, "1m")
	//beginWebhook          = flag.String("bw", "", "job begin webhook")
	//endWebhook            = flag.String("ew", "", "job end webhook")
	help = flag.Bool("h", false, "help")
)

type Response struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Error  error  `json:"error"`
}

func SendError(resp *Response) {
	SendResponse(resp)
	os.Exit(-1)
}

func SendSuccess(resp *Response) {
	SendResponse(resp)
	os.Exit(0)
}

func SendResponse(resp *Response) {
	reply := map[string]interface{}{
		"stdout": resp.Stdout,
		"stderr": resp.Stderr,
	}
	if resp.Error != nil {
		reply["error"] = map[string]string{
			"message": resp.Error.Error(),
		}
	}
	b, _ := json.Marshal(reply)
	fmt.Println(string(b))
}

func main() {
	flag.Parse()
	if *help {
		fmt.Println("k8s-cronjob [options] command in container")
		return
	}
	if *labels == "" && *podName == "" {
		SendError(&Response{
			Error: fmt.Errorf("labels and pod name all empty"),
		})
	}
	cmd := flag.Args()
	config, err := rest.InClusterConfig()
	if err != nil {
		SendError(&Response{
			Error: fmt.Errorf("load cluster config error: %v", err),
		})
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		SendError(&Response{
			Error: fmt.Errorf("create cluster client error: %v", err),
		})
	}
	var (
		runningPodName string
	)
	if *waitRunningPodTimeout > 0 {
		runningPodName, err = LookupRunningPodTimeout(clientset, *namespace, *labels, *podName, *containerName, *waitRunningPodTimeout)
	} else {
		runningPodName, err = LookupRunningPod(clientset, *namespace, *labels, *podName, *containerName)
	}
	if err != nil {
		SendError(&Response{
			Error: fmt.Errorf("lookup running pod error: %v", err),
		})
	}
	stdoutStr, stderrStr, err := ExecInPod(clientset, config, *namespace, runningPodName, *containerName, cmd)
	if err != nil {
		SendError(&Response{
			Stdout: stdoutStr,
			Stderr: stderrStr,
			Error:  err,
		})
	}
	SendSuccess(&Response{
		Stdout: stdoutStr,
		Stderr: stderrStr,
		Error:  nil,
	})
}

func LookupRunningPodTimeout(clientset *kubernetes.Clientset, namespace string, labels string, podName string, containerName string, timeout time.Duration) (string, error) {
	start := time.Now()
	for {
		podName, err := LookupRunningPod(clientset, namespace, labels, podName, containerName)
		if err == nil {
			return podName, nil
		}
		if time.Since(start) > timeout {
			return "", fmt.Errorf("lookup running pod timeout")
		}
		time.Sleep(time.Second * 5)
	}
}

func LookupRunningPod(clientset *kubernetes.Clientset, namespace string, labels string, podName string, containerName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	if podName != "" {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, v1.GetOptions{})
		if err != nil {
			return "", err
		}
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
		return "", fmt.Errorf("no running pod found")
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels,
	})
	if err != nil {
		return "", err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("no running pod found")
}

func ExecInPod(clientset *kubernetes.Clientset, config *rest.Config, namespace string, podName string, containerName string, cmd []string) (string, string, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).SubResource("exec")
	if containerName != "" {
		req = req.Param("container", containerName)
	}
	req.VersionedParams(
		&corev1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		},
		scheme.ParameterCodec,
	)

	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())
	if err != nil {
		return stdoutStr, stderrStr, err
	}
	if stderrStr != "" {
		return stdoutStr, stderrStr, fmt.Errorf(stderrStr)
	}
	return stdoutStr, stderrStr, nil

}
