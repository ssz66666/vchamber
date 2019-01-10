package schedule

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/go-redis/redis"
	"k8s.io/client-go/rest"
)

type Orchestrator struct {
	store  Storage
	client *redis.Client
}

func NewOrchestrator(rclient *redis.Client, s Storage) *Orchestrator {

	return &Orchestrator{
		store:  s,
		client: rclient,
	}
}

func (o *Orchestrator) UpdateBackendInfo(clientset *kubernetes.Clientset) {
	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{LabelSelector: "tier=backend"})
	if err != nil {
		panic(err)
	}
	npods := len(pods.Items)

	b := make(map[Backend]ServerLoad)

	for i := 0; i < npods; i++ {
		host := fmt.Sprintf("vc-backend-%d.ws-backend-service:8080", i)
		if rsp, err := http.Get("http://" + host + "/server"); err == nil {
			buf, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				continue
			}
			var m server.ServerInfoMsg
			err = json.Unmarshal(buf, &m)
			if err != nil {
				continue
			}
			for j := 0; j < len(m.Rooms); j++ {
				o.store.Set(m.Rooms[j], host)
			}
		}
		b[Backend(host)] = 1.0
	}

	msg, _ := json.Marshal(&ScheduleInfo{
		Backends: b,
		Strategy: SchedulingStrategyBalance,
	})
	if err := o.client.Publish(SchedulePubSubChannel, string(msg)).Err(); err != nil {
		panic(err)
	}
}

func (o *Orchestrator) Run() {
	ticker := time.NewTicker(SchedulingUpdatePeriod)
	defer func() {
		ticker.Stop()
		o.client.Close()
	}()

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	o.UpdateBackendInfo(clientset)
	for {
		select {
		case <-ticker.C:
			o.UpdateBackendInfo(clientset)
		}
	}
}
