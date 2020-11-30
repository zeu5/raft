package main

type LogEntry struct {
	command *Command
	term    int
	index   int
}

type Store interface {
	AppendLog(*LogEntry)
	GetLogs() []*LogEntry
	LogAt(int) *LogEntry
	ClearFrom(int)
	Slice(int, int) []*LogEntry
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

func (m *MemStore) ClearFrom(_ int) {

}

func (m *MemStore) Slice(_ int, _ int) []*LogEntry {
	return nil
}
