package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector"
	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector/deltap"
)

type curlWriter struct {
	W io.Writer
}

func (w *curlWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(w.W, "# %s\n", p)
}

func main() {
	var address string
	flag.StringVar(&address, "address", "http://localhost:5000", "DifferentialPressureAPI server address")
	var sensorName string
	flag.StringVar(&sensorName, "sensorName", "", "Sensor name")
	var nodeName string
	flag.StringVar(&nodeName, "nodeName", "", "Node name")
	var nodeIP string
	flag.StringVar(&nodeIP, "nodeIP", "", "Node IP address")
	var basicAuth string
	flag.StringVar(&basicAuth, "basicAuth", "", "Basic auth in username@password format")
	flag.Parse()

	requestEditorFns := []metriccollector.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, metriccollector.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, metriccollector.WithCurlLogger(&curlWriter{W: log.Writer()}))

	c := deltap.NewDifferentialPressureAPIClient(address, sensorName, nodeName, nodeIP, true, 2*time.Second, requestEditorFns...)

	v, err := c.Fetch(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(v)
}
