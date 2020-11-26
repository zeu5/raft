package main

import (
	"encoding/json"
	"fmt"
)

type Message interface {
	Type() string
	Marshall() []byte
	Unmarshall([]byte)
}

type AppendEntriesReq struct {
	term         int    `json:"term"`
	leaderId     int    `json:"leader_id"`
	prevLogIndex int    `json:"prev_log_index"`
	prevLogTerm  int    `json:"prev_log_term"`
	command      string `json:"command"`
	leaderCommit int    `json:"leader_commit"`
}

func (a *AppendEntriesReq) Marshall() []byte {
	res, err := json.Marshal(a)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (a *AppendEntriesReq) Unmarshall(data []byte) {
	if err := json.Unmarshal(data, a); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

func (a *AppendEntriesReq) Type() string {
	return "AppendEntriesReq"
}

type AppendEntriesReply struct {
	term    int  `json:"term"`
	success bool `json:"success"`
}

func (a *AppendEntriesReply) Marshall() []byte {
	res, err := json.Marshal(a)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (a *AppendEntriesReply) Unmarshall(data []byte) {
	if err := json.Unmarshal(data, a); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

func (a *AppendEntriesReply) Type() string {
	return "AppendEntriesReply"
}

type RequestVoteReq struct {
	term         int `json:"term"`
	candidateId  int `json:"candidate_id"`
	lastLogIndex int `json:"last_log_index"`
	lastLogTerm  int `json:"last_log_term"`
}

func (r *RequestVoteReq) Type() string {
	return "RequestVoteReq"
}

func (r *RequestVoteReq) Marshall() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *RequestVoteReq) Unmarshall(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type RequestVoteReply struct {
	term int  `json:"term"`
	vote bool `json:"vote"`
}

func (r *RequestVoteReply) Type() string {
	return "RequestVoteReply"
}

func (r *RequestVoteReply) Marshall() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *RequestVoteReply) Unmarshall(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type ClientRequest struct {
	ClientAddr string
	command    *Command
}

func (r *ClientRequest) Type() string {
	return "ClientRequest"
}

func (r *ClientRequest) Marshall() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *ClientRequest) Unmarshall(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type Transport interface {
	Run()
	ReceiveChan() <-chan Message
	SendAppendEntries(*Peer, *AppendEntriesReq) error
	SendRequestVote(*Peer, *RequestVoteReq) error
	ReplyAppendEntries(*Peer, *AppendEntriesReply) error
	ReplyRequestVote(*Peer, *RequestVoteReply) error
	ReplyClient(string, string)
}
