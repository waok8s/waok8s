package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/waok8s/wao-core/pkg/metrics/inlettemp"
	"github.com/waok8s/wao-core/pkg/util"
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

	requestEditorFns := []util.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, util.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, util.WithCurlLogger(&curlWriter{W: log.Writer()}))

	c := inlettemp.NewRedfishClient(address, inlettemp.ServerType(serverType), true, 2*time.Second, requestEditorFns...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	v, err := c.Fetch(ctx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(v)
}
