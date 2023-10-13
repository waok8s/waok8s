package collector

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metric"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

type Collector struct {
	client      client.Client
	metricStore *metric.Store
}

func New(cfg *rest.Config, metricStore *metric.Store) (*Collector, error) {
	ca, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	go ca.Start(context.TODO())
	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: ca},
	})
	if err != nil {
		return nil, err
	}

	return &Collector{
		client:      c,
		metricStore: metricStore,
	}, nil
}

func (c *Collector) Run(stopCh <-chan struct{}) error {
	// TODO
	<-stopCh
	return nil
}
