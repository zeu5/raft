package raft

import (
	"encoding/json"
	"fmt"
)

type Command struct {
	data []byte
}

type FSM interface {
	ApplyCommand(*Command) string
}

type KeyValueCommand struct {
	Op string `json:"op"`
	K  string `json:"key"`
	V  string `json:"value"`
}

func (c *KeyValueCommand) Marshal() []byte {
	v, err := json.Marshal(c)
	if err != nil {
		panic(fmt.Errorf("Could not marshall command"))
	}
	return v
}

func (c *KeyValueCommand) Unmarshal(s []byte) {
	err := json.Unmarshal(s, c)
	if err != nil {
		panic(fmt.Errorf("Could not unmarshall command"))
	}
}

type KeyValueFSM struct {
	m map[string]string
}

func NewKeyValueFSM() *KeyValueFSM {
	return &KeyValueFSM{
		make(map[string]string),
	}
}

func (k *KeyValueFSM) ApplyCommand(c *Command) string {
	if string(c.data) == "no-op" {
		return ""
	}
	command := &KeyValueCommand{}
	command.Unmarshal(c.data)

	switch command.Op {
	case "Get":
		v, exists := k.m[command.K]
		if !exists {
			return ""
		}
		return v
	case "Set":
		k.m[command.K] = command.V
		return "OK"
	default:
		return ""
	}
}
