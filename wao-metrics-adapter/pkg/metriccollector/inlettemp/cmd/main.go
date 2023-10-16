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
	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector/inlettemp"
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
	var serverType string
	var serverTypeUsage strings.Builder
	serverTypeUsage.WriteString("Options:")
	for k := range inlettemp.GetSensorValueFn {
		serverTypeUsage.WriteString(" " + string(k))
	}
	flag.StringVar(&serverType, "serverType", "", serverTypeUsage.String())
	var basicAuth string
	flag.StringVar(&basicAuth, "basicAuth", "", "Basic auth in username@password format")
	flag.Parse()

	c := inlettemp.NewRedfishClient(address, inlettemp.ServerType(serverType), true, 2*time.Second)

	requestEditorFns := []metriccollector.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, metriccollector.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, metriccollector.WithCurlLogger(&curlWriter{W: log.Writer()}))

	v, err := c.Fetch(context.TODO(), requestEditorFns...)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(v)
}
