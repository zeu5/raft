package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type Client struct {
	masterAddr string
}

func NewClient(c *Config) *Client {
	return &Client{
		masterAddr: c.MasterAddr,
	}
}

func (c *Client) Run() {
	leader := c.fetchLeader()
	if leader != "" {
		c.sendWorkload(leader)
	}
}

func (c *Client) fetchLeader() string {
	url := "http://" + c.masterAddr + "/leader"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic("Could not fetch leader address")
	}
	resp, err := client.Do(req)
	if err != nil {
		panic("Could not fetch leader address")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic("Could not fetch leader address")
	}
	fmt.Println(string(body))
	return string(body)
}

func (c *Client) sendWorkload(leader string) {
	command1 := &KeyValueCommand{
		Op: "Set",
		K:  "First",
		V:  "v1",
	}
	command2 := &KeyValueCommand{
		Op: "Set",
		K:  "First",
		V:  "v2",
	}
	c.sendMsg(leader, &ClientRequest{
		Command: string(command1.Marshal()),
	})
	c.sendMsg(leader, &ClientRequest{
		Command: string(command2.Marshal()),
	})
}

func (c *Client) sendMsg(addr string, r *ClientRequest) {
	body, err := json.Marshal(&TransportMessage{
		Type:    r.Type(),
		Message: string(r.Marshal()),
	})
	if err != nil {
		log.Println("Could not marshal req")
		return
	}
	url := "http://" + addr
	client := &http.Client{}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Sending request failed", err)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err == nil {
		log.Printf("Response: %s\n", body)
	}
	resp.Body.Close()
}
