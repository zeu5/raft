package main

type Command interface {
	Marshall() string
	UnMarshall(string)
}

type FSM interface {
	ApplyCommand(c Command)
}

type KeyValueFSM struct {
	m map[string]string
}

func NewKeyValueFSM() *KeyValueFSM {
	return &KeyValueFSM{
		make(map[string]string),
	}
}

func (k *KeyValueFSM) ApplyCommand(_ Command) {}
