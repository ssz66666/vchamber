package main

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	for {
		// pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
		// s, err := clientset.CoreV1().Services("").Get("backend-service", metav1.GetOptions{})
		// set := labels.Set(s.Spec.Selector)
		pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{LabelSelector: "app=vchamber"})
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("There are %d pods in the svc %v\n", len(pods.Items), "backend-service")
		for _, pod := range pods.Items {
			fmt.Printf("pod name: %v\n", pod.Name)
		}
		time.Sleep(5 * time.Second)
	}
}
