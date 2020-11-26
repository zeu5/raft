package main

type LogEntry struct {
	command *Command
	term    int
	index   int
}

type Store interface {
	AppendLog(*LogEntry)
	UpdateState(*State)
	GetLogs() []*LogEntry
	LogAt(int) *LogEntry
	ClearFrom(int)
}

type MemStore struct {
}

func NewMemStore(_ *Config) *MemStore {
	return &MemStore{}
}

func (m *MemStore) GetLogs() []*LogEntry {
	return nil
}

func (m *MemStore) LogAt(index int) *LogEntry {
	return nil
}

func (m *MemStore) AppendLog(_ *LogEntry) {

}

func (m *MemStore) UpdateState(_ *State) {

}

func (m *MemStore) ClearFrom(_ int) {

}
