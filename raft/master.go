package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Controller interface {
	NotifyMessage(*MessageWrapper)
	ReceiveChan() chan *MessageWrapper
	Run()
}

type Master struct {
	controller Controller
	recvChan   chan *MessageWrapper
	peers      *PeerStore
	serverAddr string
}

func NewMaster(c *Config) *Master {
	peers := NewPeerStore(c)
	ctr := NewPCTFileController(c)
	return &Master{
		peers:      peers,
		controller: ctr,
		recvChan:   ctr.ReceiveChan(),
		serverAddr: c.MasterAddr,
	}
}

func (m *Master) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	defer fmt.Fprintf(w, "OK")
	if r.Method != "POST" {
		return
	}
	d := json.NewDecoder(r.Body)
	var msg MessageWrapper
	if err := d.Decode(&msg); err == nil {
		m.controller.NotifyMessage(&msg)
	}
	r.Body.Close()
}

func (m *Master) Monitor() {
	for {
		select {
		case msg := <-m.recvChan:
			m.sendMsg(msg)
		default:
		}
	}
}

func (m *Master) sendMsg(msg *MessageWrapper) error {
	p := m.peers.GetPeer(msg.To)

	body, err := json.Marshal(&msg.M)
	if err != nil {
		fmt.Printf("Could not marshall message %s\n", err)
		return nil
	}

	url := "http://" + p.addr
	client := &http.Client{}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Sending request failed", err)
		return fmt.Errorf("Request failed %s", err)
	}
	resp.Body.Close()
	return nil
}

func (m *Master) Run() {
	go m.controller.Run()
	go m.Monitor()

	http.HandleFunc("/", m.HandlerFunc)
	http.ListenAndServe(m.serverAddr, nil)

}

type MessageWrapper struct {
	From int              `json:"from"`
	To   int              `json:"to"`
	M    TransportMessage `json:"m"`
}

type TimeoutController struct {
	in  chan *MessageWrapper
	out chan *MessageWrapper
}

func (c *TimeoutController) NotifyMessage(m *MessageWrapper) {
	if m.M.Type != "Timeout" {
		c.out <- m
		return
	}
	c.in <- m
}

func (c *TimeoutController) ReceiveChan() chan *MessageWrapper {
	return c.out
}

func (c *TimeoutController) AsyncTimeout(m *MessageWrapper, d time.Duration) {
	time.Sleep(d)
	c.out <- m
}

func (c *TimeoutController) Run() {
	for {
		select {
		case m := <-c.in:
			duration := time.Duration(randIntn(2000)) * time.Millisecond
			go c.AsyncTimeout(m, duration)
		default:
		}
	}
}

func NewTimeoutController(_ *Config) *TimeoutController {
	return &TimeoutController{
		out: make(chan *MessageWrapper, 100),
		in:  make(chan *MessageWrapper, 100),
	}
}
