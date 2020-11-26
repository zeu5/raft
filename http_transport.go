package main

import "net/http"

type HTTPTransport struct {
	recvChan chan Message
}

func NewHTTPTransport(_ *Config) *HTTPTransport {
	return &HTTPTransport{
		make(chan Message, 100),
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
