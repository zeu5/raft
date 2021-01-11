package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
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
	leaderAddr string
	term       int
	lock       *sync.Mutex
	server     *http.Server
}

func NewMaster(c *Config) *Master {
	peers := NewPeerStore(c)
	ctr := NewStructuredTimeoutController(c)
	m := &Master{
		peers:      peers,
		controller: ctr,
		recvChan:   ctr.ReceiveChan(),
		serverAddr: c.MasterAddr,
		lock:       new(sync.Mutex),
		term:       -1,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.HandlerFunc)
	mux.HandleFunc("/leader", m.LeaderHandler)

	m.server = &http.Server{
		Addr:    m.serverAddr,
		Handler: mux,
	}
	return m
}

type LeaderPing struct {
	Addr string `json:"addr"`
	ID   int    `json:"id"`
	Term int    `json:"term"`
}

func (m *Master) LeaderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {

		d := json.NewDecoder(r.Body)
		var l LeaderPing
		if err := d.Decode(&l); err == nil {
			m.lock.Lock()
			if m.term == -1 {
				fmt.Println("Replicas ready")
			}
			if l.Term > m.term {
				m.leaderAddr = l.Addr
				m.term = l.Term
			}
			m.lock.Unlock()
		}
		r.Body.Close()
		fmt.Fprintf(w, "OK")
		return
	} else if r.Method == "GET" {
		m.lock.Lock()
		leader := m.leaderAddr
		m.lock.Unlock()
		fmt.Fprintf(w, "%s", leader)
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
		// log.Printf("Received message %#v\n", msg)
		m.controller.NotifyMessage(&msg)
	}
	r.Body.Close()
}

func (m *Master) Monitor() {
	for {
		select {
		case msg := <-m.recvChan:
			m.sendMsg(msg)
		}
	}
}

func (m *Master) sendMsg(msg *MessageWrapper) error {
	p := m.peers.GetPeer(msg.To)
	body, err := json.Marshal(&msg.M)
	if err != nil {
		// fmt.Printf("Could not marshall message %s\n", err)
		return nil
	}
	// log.Printf("Sending message %s to %d from %d", body, msg.To, msg.From)
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

func (m *Master) Run(ctx context.Context) {
	log.Println("Starting master")
	go m.controller.Run()
	go m.Monitor()

	go func() {
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %+s\n", err)
		}
	}()

	<-ctx.Done()
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer func() {
		cancel()
	}()

	if err := m.server.Shutdown(shutDownCtx); err != nil {
		log.Fatalf("Server shutdown failed:%+s\n", err)
	}
}

type MessageWrapper struct {
	From int              `json:"from"`
	To   int              `json:"to"`
	M    TransportMessage `json:"m"`
	ID   int              `json:"id"`
}

type TimeoutController struct {
	counter int
	lock    *sync.Mutex
	out     chan *MessageWrapper
}

func (c *TimeoutController) NotifyMessage(m *MessageWrapper) {
	c.lock.Lock()
	m.ID = c.counter
	c.counter = c.counter + 1
	c.lock.Unlock()

	b, _ := json.Marshal(m)
	log.Printf("Received: %s\n", b)

	if m.M.Type != "Timeout" {
		log.Printf("Sending: %s\n", b)
		c.out <- m
		return
	}
	timeout := &TimeoutMessage{}
	timeout.Unmarshal([]byte(m.M.Message))
	duration := time.Duration(randIntn(2000)) * time.Millisecond

	log.Printf("Waiting for %d millis for %#v", duration.Milliseconds(), m)
	timeout.Time = duration.Milliseconds()
	m.M.Message = string(timeout.Marshal())

	time.Sleep(duration)
	log.Printf("Sending: %s\n", b)
	c.out <- m
}

func (c *TimeoutController) ReceiveChan() chan *MessageWrapper {
	return c.out
}

func (c *TimeoutController) Run() {
}

func NewTimeoutController() *TimeoutController {
	return &TimeoutController{
		out:     make(chan *MessageWrapper, 100),
		counter: 0,
		lock:    new(sync.Mutex),
	}
}

type RandomWalkController struct {
	out     chan *MessageWrapper
	pool    map[int]*MessageWrapper
	counter int
	lock    *sync.Mutex
}

func NewRandomWalkController() *RandomWalkController {
	return &RandomWalkController{
		out:     make(chan *MessageWrapper, 100),
		pool:    make(map[int]*MessageWrapper),
		counter: 0,
		lock:    new(sync.Mutex),
	}
}

func (r *RandomWalkController) NotifyMessage(m *MessageWrapper) {
	r.lock.Lock()
	r.pool[r.counter] = m
	m.ID = r.counter
	r.counter = r.counter + 1
	r.lock.Unlock()

	if b, err := json.Marshal(m); err == nil {
		log.Printf("Received: %s\n", b)
	}
}

func (r *RandomWalkController) ReceiveChan() chan *MessageWrapper {
	return r.out
}

func (r *RandomWalkController) Run() {
	ticker := time.NewTicker(20 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			var m *MessageWrapper
			send := false
			r.lock.Lock()
			rand := randIntn(r.counter + 1)
			if msg, ok := r.pool[rand]; ok {
				m = msg
				send = true
				delete(r.pool, rand)
			}
			r.lock.Unlock()

			if send {
				r.out <- m
				if b, err := json.Marshal(m); err == nil {
					log.Printf("Sending: %s\n", b)
				}
			}
		}
	}
}
