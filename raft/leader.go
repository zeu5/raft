package raft

import (
	"log"
	"sync"
)

type MatchIndices struct {
	matchIndices map[int]int
	lock         *sync.Mutex
	commitCh     chan struct{}
	quorumSize   int
	commitIndex  int
	minIndex     int
}

func NewMatchIndices(p *PeerStore, commitCh chan struct{}, quorumSize, commitIndex, minIndex int) *MatchIndices {
	m := &MatchIndices{
		matchIndices: make(map[int]int),
		lock:         new(sync.Mutex),
		commitCh:     commitCh,
		quorumSize:   quorumSize,
		commitIndex:  commitIndex,
		minIndex:     minIndex,
	}
	for id, _ := range p.AllPeers() {
		m.matchIndices[id] = 0
	}
	return m
}

func (m *MatchIndices) Reset(c, i int, ch chan struct{}) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for id, _ := range m.matchIndices {
		m.matchIndices[id] = 0
	}
	m.commitIndex = c
	m.minIndex = i
	m.commitCh = ch
}

func (m *MatchIndices) Update(replica, index int) {
	m.lock.Lock()
	defer m.lock.Unlock()
	v, ok := m.matchIndices[replica]
	if ok && index > v {
		m.matchIndices[replica] = index
		m.recalculate()
	}
}

func (m *MatchIndices) recalculate() {
	indices := make(map[int]int)
	for _, i := range m.matchIndices {
		if _, ok := indices[i]; ok {
			indices[i] = indices[i] + 1
		} else {
			indices[i] = 1
		}
	}

	flag := false
	for index, count := range indices {
		if count >= m.quorumSize && index > m.commitIndex && index >= m.minIndex {
			m.commitIndex = index
			flag = true
		}
	}

	if flag {
		asyncNotify(m.commitCh)
		log.Printf("Recalculated new index! %d", m.commitIndex)
	}
}

func (m *MatchIndices) CommitIndex() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.commitIndex
}

type ReplicationState struct {
	nextIndex  int
	peer       *Peer
	stopChan   chan bool
	matchIndex *MatchIndices
	lock       *sync.Mutex
}

func NewReplicationState(peer *Peer, nextIndex int, m *MatchIndices) *ReplicationState {
	return &ReplicationState{
		nextIndex:  nextIndex,
		peer:       peer,
		matchIndex: m,
		stopChan:   make(chan bool, 1),
		lock:       new(sync.Mutex),
	}
}

func (r *ReplicationState) UpdateReply(success bool, index int) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if success {
		log.Printf("Got success reply from %d\n", r.peer.id)
		r.matchIndex.Update(r.peer.id, index)
	} else {
		r.nextIndex = max(min(r.nextIndex-1, index), 1)
	}
}

func (r *ReplicationState) NextIndex() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.nextIndex
}

func (r *ReplicationState) IncNextIndex() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.nextIndex = r.nextIndex + 1
}

type LeaderState struct {
	replicationState map[int]*ReplicationState
	lock             *sync.Mutex
	matchIndices     *MatchIndices
	peerStore        *PeerStore
	state            *State
	incoming         chan *Command
	commitCh         chan struct{}
}

func NewLeaderState(N int, peerStore *PeerStore, state *State) *LeaderState {
	lastIndex, _ := state.LastLog()
	s := &LeaderState{
		lock:             new(sync.Mutex),
		peerStore:        peerStore,
		state:            state,
		incoming:         make(chan *Command, 100),
		commitCh:         make(chan struct{}, 1),
		replicationState: make(map[int]*ReplicationState),
	}
	s.matchIndices = NewMatchIndices(
		peerStore,
		s.commitCh,
		N/2+1,
		state.CommitIndex(),
		lastIndex,
	)
	for id, peer := range peerStore.AllPeers() {
		s.replicationState[id] = NewReplicationState(peer, lastIndex+1, s.matchIndices)
	}
	return s
}

func (l *LeaderState) Reset() {
	l.lock.Lock()
	defer l.lock.Unlock()
	lastIndex, _ := l.state.LastLog()
	for id, peer := range l.peerStore.AllPeers() {
		l.replicationState[id] = NewReplicationState(peer, lastIndex+1, l.matchIndices)
	}
	l.commitCh = make(chan struct{}, 1)
	l.matchIndices.Reset(l.state.CommitIndex(), lastIndex, l.commitCh)
	l.incoming = make(chan *Command, 100)
}

func (l *LeaderState) Stop() {
	for _, repl := range l.replicationState {
		repl.stopChan <- true
	}
}

func (l *LeaderState) UpdateReplicationStatus(replica int, success bool, lastLog int) {
	l.replicationState[replica].UpdateReply(success, lastLog)
}

func (n *Node) leaderLoop() {

	for _, r := range n.leaderState.replicationState {
		go n.replicate(r)
	}

	for n.state.RaftState() == Leader {
		select {
		case c := <-n.leaderState.incoming:
			log.Printf("Storing log %#v\n", c)
			logE := &LogEntry{
				command: c,
				index:   n.state.LastLogIndex() + 1,
				term:    n.state.CurrentTerm(),
			}
			n.store.AppendLog(logE)
			n.state.UpdateLastLog(logE)
			n.leaderState.matchIndices.Update(n.id, logE.index)
		case <-n.leaderState.commitCh:
			n.state.SetCommitIndex(n.leaderState.matchIndices.CommitIndex())
		}
	}
}

func (n *Node) replicate(r *ReplicationState) {
	for {
		select {
		case <-r.stopChan:
			return
		default:
		}

		nextIndex := r.NextIndex()
		if nextIndex <= n.state.LastLogIndex() {
			curLog := n.store.LogAt(nextIndex)
			req := &AppendEntriesReq{
				Term:         n.state.CurrentTerm(),
				LeaderId:     n.id,
				Command:      string(curLog.command.data),
				LeaderCommit: n.state.CommitIndex(),
			}
			if nextIndex == 1 {
				req.PrevLogIndex = 0
				req.PrevLogTerm = 0
			} else {
				prevLog := n.store.LogAt(nextIndex - 1)
				req.PrevLogIndex = prevLog.index
				req.PrevLogTerm = prevLog.term
			}
			n.transport.SendAppendEntries(r.peer, req)
			r.IncNextIndex()
		}
	}
}
