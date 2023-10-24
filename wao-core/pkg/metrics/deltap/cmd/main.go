package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/waok8s/wao-core/pkg/metrics/deltap"
	"github.com/waok8s/wao-core/pkg/util"
)

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

	requestEditorFns := []util.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, util.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, util.WithCurlLogger(&util.CurlWriter{W: log.Writer()}))

	c := deltap.NewDifferentialPressureAPIClient(address, sensorName, nodeName, nodeIP, true, 2*time.Second, requestEditorFns...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	v, err := c.Fetch(ctx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(v)
}
