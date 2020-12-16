package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"

	"github.com/zeu5/raft/raft"
)

var master *bool = flag.Bool("master", false, "Decides to run the node as a master node")
var client *bool = flag.Bool("client", false, "Runs the workload client")
var config *string = flag.String("conf", "", "Config file path")

func openConfFile(path string) []byte {
	s, err := ioutil.ReadFile(path)
	if err != nil {
		panic("Could not open config file")
	}
	return s
}

func main() {

	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		oscall := <-termCh
		log.Printf("Received syscall: %#v", oscall)
		cancel()
	}()

	flag.Parse()
	if *master && *client {
		panic(fmt.Errorf("Use only one of master or client"))
	}
	config := raft.ConfigFromJson(openConfFile(*config))
	if *master {
		m := raft.NewMaster(config)
		m.Run(ctx)
	} else if *client {
		m := raft.NewClient(config)
		m.Run()
	} else {
		n := raft.NewNode(config)
		n.Run(ctx)
	}
}
