package main

type Store interface {
	AppendLog(Command)
	UpdateState(*State)
	GetLogs() []Command
	LogAt(int) Command
}

type MemStore struct {
}

func NewMemStore(_ *Config) *MemStore {
	return &MemStore{}
}

func (m *MemStore) GetLogs() []Command {
	return nil
}

func (m *MemStore) LogAt(index int) Command {
	return nil
}

func (m *MemStore) AppendLog(_ Command) {
}

func (m *MemStore) UpdateState(_ *State) {
}
