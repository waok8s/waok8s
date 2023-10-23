package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/waok8s/wao-metrics-adapter/pkg/metriccollector"
	"github.com/waok8s/wao-scheduler/pkg/predictor/endpointprovider"
)

type curlWriter struct {
	W io.Writer
}

func (w *curlWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(w.W, "# %s\n", p)
}

func main() {
	var address string
	flag.StringVar(&address, "address", "http://localhost:5000", "Redfish server address")
	var basicAuth string
	flag.StringVar(&basicAuth, "basicAuth", "", "Basic auth in username@password format")
	flag.Parse()

	requestEditorFns := []metriccollector.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, metriccollector.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, metriccollector.WithCurlLogger(&curlWriter{W: log.Writer()}))

	c, err := endpointprovider.NewRedfishEndpointProvider(address, true, 2*time.Second, requestEditorFns...)
	if err != nil {
		log.Fatal(err)
	}

	models, err := c.GetModels(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", models)
}
