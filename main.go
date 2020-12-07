package main

import (
	"flag"
	"io/ioutil"

	"github.com/zeu5/raft/raft"
)

var master *bool = flag.Bool("master", false, "Decides to run the node as a master node")
var config *string = flag.String("conf", "", "Config file path")

func openConfFile(path string) []byte {
	s, err := ioutil.ReadFile(path)
	if err != nil {
		panic("Could not open config file")
	}
	return s
}

func main() {
	flag.Parse()
	config := raft.ConfigFromJson(openConfFile(*config))
	if *master {
		m := raft.NewMaster(config)
		m.Run()
	} else {
		n := raft.NewNode(config)
		n.Run()
	}
}
