package main

import "sync"

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
	logs []*LogEntry
	size int
	lock *sync.Mutex
}

func NewMemStore(_ *Config) *MemStore {
	return &MemStore{
		logs: make([]*LogEntry, 0),
		size: 0,
		lock: new(sync.Mutex),
	}
}

func (m *MemStore) GetLogs() []*LogEntry {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.logs
}

func (m *MemStore) LogAt(index int) *LogEntry {
	m.lock.Lock()
	defer m.lock.Unlock()
	if index < 1 || index > m.size {
		return nil
	}
	return m.logs[index]
}

func (m *MemStore) AppendLog(l *LogEntry) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.logs = append(m.logs, l)
	m.size = m.size + 1
}

func (m *MemStore) ClearFrom(index int) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.logs = m.logs[:index+1]
	m.size = len(m.logs)
}

func (m *MemStore) Slice(from int, to int) (logs []*LogEntry) {
	if from < 1 || from > m.size || to < from || to < 1 {
		return
	}
	logs = m.logs[from:to]
	return
}
