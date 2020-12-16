package raft

import (
	"context"
	"encoding/json"
	"fmt"
)

type Message interface {
	Type() string
	Marshal() []byte
	Unmarshal([]byte)
}

type AppendEntriesReq struct {
	Term         int    `json:"term"`
	LeaderId     int    `json:"leader_id"`
	PrevLogIndex int    `json:"prev_log_index"`
	PrevLogTerm  int    `json:"prev_log_term"`
	Command      string `json:"command"`
	LeaderCommit int    `json:"leader_commit"`
}

func (a *AppendEntriesReq) Marshal() []byte {
	res, err := json.Marshal(a)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (a *AppendEntriesReq) Unmarshal(data []byte) {
	if err := json.Unmarshal(data, a); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

func (a *AppendEntriesReq) Type() string {
	return "AppendEntriesReq"
}

type AppendEntriesReply struct {
	ReplicaID int  `json:"replica_id"`
	Term      int  `json:"term"`
	Success   bool `json:"success"`
	LastLog   int  `json:"last_log"`
	HeartBeat bool `json:"heartbeat"`
}

func (a *AppendEntriesReply) Marshal() []byte {
	res, err := json.Marshal(a)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (a *AppendEntriesReply) Unmarshal(data []byte) {
	if err := json.Unmarshal(data, a); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

func (a *AppendEntriesReply) Type() string {
	return "AppendEntriesReply"
}

type RequestVoteReq struct {
	Term         int `json:"term"`
	CandidateId  int `json:"candidate_id"`
	LastLogIndex int `json:"last_log_index"`
	LastLogTerm  int `json:"last_log_term"`
}

func (r *RequestVoteReq) Type() string {
	return "RequestVoteReq"
}

func (r *RequestVoteReq) Marshal() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *RequestVoteReq) Unmarshal(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type RequestVoteReply struct {
	ReplicaID int  `json:"replica_id"`
	Term      int  `json:"term"`
	Vote      bool `json:"vote"`
	ForTerm   int  `json:"for_term"`
}

func (r *RequestVoteReply) Type() string {
	return "RequestVoteReply"
}

func (r *RequestVoteReply) Marshal() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *RequestVoteReply) Unmarshal(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type ClientRequest struct {
	Command string `json:"command"`
	resp    chan *ClientResponse
}

type ClientResponse struct {
	text       string
	statuscode int
}

func (r *ClientRequest) Type() string {
	return "ClientRequest"
}

func (r *ClientRequest) Marshal() []byte {
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Errorf("Could not marshall req")
	}
	return res
}

func (r *ClientRequest) Unmarshal(data []byte) {
	if err := json.Unmarshal(data, r); err != nil {
		fmt.Errorf("Failed to unmarshall")
	}
}

type Transport interface {
	Run(context.Context)
	ReceiveChan() <-chan Message
	SendAppendEntries(*Peer, *AppendEntriesReq) error
	SendRequestVote(*Peer, *RequestVoteReq) error
	ReplyAppendEntries(*Peer, *AppendEntriesReply) error
	ReplyRequestVote(*Peer, *RequestVoteReply) error
	SendLeaderPing(*LeaderPing) error
}
