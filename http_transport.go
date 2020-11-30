package main

import "net/http"

type HTTPTransport struct {
	recvChan  chan Message
	peerStore *PeerStore
}

func NewHTTPTransport(_ *Config, p *PeerStore) *HTTPTransport {
	return &HTTPTransport{
		recvChan:  make(chan Message, 100),
		peerStore: p,
	}
}

func (t *HTTPTransport) Run() {
	http.ListenAndServe(":5050", nil)
}

func (t *HTTPTransport) ReceiveChan() <-chan Message {
	return t.recvChan
}

func (t *HTTPTransport) SendAppendEntries(_ *Peer, _ *AppendEntriesReq) error {
	return nil
}

func (t *HTTPTransport) SendRequestVote(_ *Peer, _ *RequestVoteReq) error {
	return nil
}

func (t *HTTPTransport) ReplyAppendEntries(_ *Peer, _ *AppendEntriesReply) error {
	return nil
}

func (t *HTTPTransport) ReplyRequestVote(_ *Peer, _ *RequestVoteReply) error {
	return nil
}

func (t *HTTPTransport) ReplyClient(addr, msg string) {

}
