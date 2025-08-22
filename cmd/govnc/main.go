package main

import (
	"fmt"
	"os"

	"github.com/codio/go-vncproxy/internal/proxyserver"
	"github.com/codio/go-vncproxy/pkg/go-vncproxy"

	"github.com/akamensky/argparse"
)

const NoVncVersion = "v1.1.1"

var indexHTML string

func main() {
	parser := argparse.NewParser("govnc", "VNCProxy for novnc")
	vncPort := parser.Int("s", "vncport", &argparse.Options{Required: false, Default: 5900, Help: "VNC port"})
	port := parser.Int("p", "port", &argparse.Options{Required: false, Default: 8080, Help: "VNC port"})
	version := parser.String("v", "version", &argparse.Options{Required: false, Default: NoVncVersion, Help: "NOVNC client version"})
	strictAuth := parser.Flag("a", "auth-strict", &argparse.Options{Required: false, Default: false, Help: "Enable strict authentication"})
	err := parser.Parse(os.Args)

	if err != nil {
		fmt.Print(parser.Usage(err))
		panic(err)
	}

	runOpts := proxyserver.RunOpts{
		VncPort:      *vncPort,
		Port:         *port,
		StrictAuth:   *strictAuth,
		NoVncVersion: *version,
	}
	err = proxyserver.Run(runOpts)
	if err != nil {
		fmt.Println("Error running VNC proxy:", err)
		os.Exit(1)
	}
}

func NewVNCProxy(vncport int) *vncproxy.Proxy {
	return proxyserver.NewVNCProxy(vncport)
}
