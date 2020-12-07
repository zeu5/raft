package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type HTTPTransport struct {
	recvChan    chan Message
	peerStore   *PeerStore
	serverAddr  string
	masterAddr  string
	timeoutChan chan *TimeoutMessage
	id          int
	slave       bool
}

type TransportMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func NewHTTPTransport(c *Config, p *PeerStore) *HTTPTransport {
	return &HTTPTransport{
		recvChan:   make(chan Message, 100),
		peerStore:  p,
		serverAddr: c.Peers[c.CurNodeIndex],
		masterAddr: c.MasterAddr,
		id:         c.CurNodeIndex,
		slave:      c.Slave,
	}
}

func (t *HTTPTransport) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	defer fmt.Fprintf(w, "Ok")
	if r.Method != "POST" {
		return
	}
	d := json.NewDecoder(r.Body)
	var m TransportMessage
	if err := d.Decode(&m); err == nil {
		// log.Printf("Received message of type %s: %s", m.Type, m.Message)
		var req Message
		timeoutMessage := false
		switch m.Type {
		case "AppendEntriesReq":
			req = &AppendEntriesReq{}
		case "AppendEntriesReply":
			req = &AppendEntriesReply{}
		case "RequestVoteReq":
			req = &RequestVoteReq{}
		case "RequestVoteReply":
			req = &RequestVoteReply{}
		case "ClientRequest":
			req = &ClientRequest{}
		case "Timeout":
			req = &TimeoutMessage{}
			timeoutMessage = true
		}
		req.Unmarshal([]byte(m.Message))
		if !timeoutMessage {
			t.recvChan <- req
		} else {
			t.timeoutChan <- req.(*TimeoutMessage)
		}
	}
	r.Body.Close()
}

func (t *HTTPTransport) TimeoutRecvChan() chan *TimeoutMessage {
	return t.timeoutChan
}

func (t *HTTPTransport) SendTimeout(m *TimeoutMessage) {
	t.sendMasterMsg(t.id, m)
}

func (t *HTTPTransport) sendMasterMsg(to int, m Message) error {
	body, err := json.Marshal(&MessageWrapper{
		From: t.id,
		To:   to,
		M: TransportMessage{
			Type:    m.Type(),
			Message: string(m.Marshal()),
		},
	})
	if err != nil {
		return fmt.Errorf("Could not format message to master %s", err)
	}
	return t.sendMsg(t.masterAddr, body)
}

func (t *HTTPTransport) Run() {
	http.HandleFunc("/", t.HandlerFunc)
	log.Printf("Starting server at %s\n", t.serverAddr)
	http.ListenAndServe(t.serverAddr, nil)
}

func (t *HTTPTransport) ReceiveChan() <-chan Message {
	return t.recvChan
}

func (t *HTTPTransport) sendMsg(addr string, body []byte) error {
	// log.Printf("Sending message %#v to %#v\n", r, p)
	url := "http://" + addr
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

func (t *HTTPTransport) SendMsg(p *Peer, m Message) error {
	if t.slave {
		return t.sendMasterMsg(p.id, m)
	}

	body, err := json.Marshal(&TransportMessage{
		Type:    m.Type(),
		Message: string(m.Marshal()),
	})
	if err != nil {
		return fmt.Errorf("Could not marshall message %s", err)
	}
	return t.sendMsg(p.addr, body)
}

func (t *HTTPTransport) SendAppendEntries(p *Peer, r *AppendEntriesReq) error {
	return t.SendMsg(p, r)
}

func (t *HTTPTransport) SendRequestVote(p *Peer, r *RequestVoteReq) error {
	return t.SendMsg(p, r)
}

func (t *HTTPTransport) ReplyAppendEntries(p *Peer, r *AppendEntriesReply) error {
	return t.SendMsg(p, r)
}

func (t *HTTPTransport) ReplyRequestVote(p *Peer, r *RequestVoteReply) error {
	return t.SendMsg(p, r)
}

func (t *HTTPTransport) ReplyClient(addr, msg string) {

}
