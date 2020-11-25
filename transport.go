package main

type Message interface {
	Type() string
	Marshall() string
	Unmarshall(string)
}

type AppendEntries struct {
}

func (a *AppendEntries) Marshall() string {
	return ""
}

func (a *AppendEntries) Unmarshall(_ string) {
}

func (a *AppendEntries) Type() string {
	return "AppendEntries"
}

type RequestVote struct {
}

func (r *RequestVote) Type() string {
	return "RequestVote"
}

func (r *RequestVote) Marshall() string {
	return ""
}

func (r *RequestVote) Unmarshall(_ string) {
}

type Transport interface {
	Run()
	ReceiveChan() <-chan Message
	SendAppendEntries(*Peer, *AppendEntries) error
	SendRequestVote(*Peer, *RequestVote) error
}
