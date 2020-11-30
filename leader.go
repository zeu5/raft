package main

import "sync"

type ReplicationState struct {
	nextIndex   int
	peer        *Peer
	stopChan    chan bool
	matchedChan chan matchedEntry
	lock        *sync.Mutex
	sent        bool
}

func NewReplicationState(peer *Peer, nextIndex int, matchedCh chan matchedEntry) *ReplicationState {
	return &ReplicationState{
		nextIndex:   nextIndex,
		peer:        peer,
		matchedChan: matchedCh,
		stopChan:    make(chan bool, 1),
		lock:        new(sync.Mutex),
		sent:        false,
	}
}

func (r *ReplicationState) UpdateReply(success bool, index int) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if success {
		r.nextIndex = r.nextIndex + 1
		r.matchedChan <- matchedEntry{r.peer.id, r.nextIndex}
	} else {
		r.nextIndex = max(min(r.nextIndex-1, index), 1)
	}
	r.sent = false
}

func (r *ReplicationState) NextIndex() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.nextIndex
}

func (r *ReplicationState) SetSent() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.sent = true
}

func (r *ReplicationState) Sent() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.sent
}

type matchedEntry struct {
	replica int
	index   int
}

type LeaderState struct {
	replicationState map[int]*ReplicationState
	lock             *sync.Mutex
	matchIndices     map[int]int
	peerStore        *PeerStore
	state            *State
	incoming         chan *Command
	matched          chan matchedEntry
	commitCh         chan bool
	minIndex         int
	commitIndex      int
	quorumSize       int
}

func NewLeaderState(N int, peerStore *PeerStore, state *State) *LeaderState {
	lastIndex, _ := state.LastLog()
	s := &LeaderState{
		lock:         new(sync.Mutex),
		matchIndices: make(map[int]int),
		peerStore:    peerStore,
		state:        state,
		incoming:     make(chan *Command, 100),
		matched:      make(chan matchedEntry, 100),
		minIndex:     lastIndex,
		commitIndex:  state.CommitIndex(),
		quorumSize:   N/2 + 1,
		commitCh:     make(chan bool, 1),
	}
	for id, peer := range peerStore.AllPeers() {
		s.replicationState[id] = NewReplicationState(peer, lastIndex, s.matched)
		s.matchIndices[id] = 0
	}
	return s
}

func (l *LeaderState) Reset() {
	l.lock.Lock()
	defer l.lock.Unlock()
	lastIndex, _ := l.state.LastLog()
	for id, peer := range l.peerStore.AllPeers() {
		l.replicationState[id] = NewReplicationState(peer, lastIndex, l.matched)
		l.matchIndices[id] = 0
	}
	l.minIndex = lastIndex
	l.commitIndex = l.state.CommitIndex()
	l.commitCh = make(chan bool, 1)
	l.incoming = make(chan *Command, 100)
	l.matched = make(chan matchedEntry, 100)
}

func (l *LeaderState) Stop() {
	for _, repl := range l.replicationState {
		repl.stopChan <- true
	}
}

func (l *LeaderState) UpdateMatched(m matchedEntry) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.matchIndices[m.replica] = m.index

	indices := make(map[int]int)
	for _, i := range l.matchIndices {
		if _, ok := indices[i]; ok {
			indices[i] = indices[i] + 1
		} else {
			indices[i] = 1
		}
	}

	for index, count := range indices {
		if count >= l.quorumSize && index > l.commitIndex && index >= l.minIndex {
			l.commitIndex = index
			l.commitCh <- true
			break
		}
	}
}

func (l *LeaderState) CommitIndex() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.commitIndex
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
		case <-n.leaderState.incoming:
		// Add to log and update lastlog
		case m := <-n.leaderState.matched:
			n.leaderState.UpdateMatched(m)
		case <-n.leaderState.commitCh:
			n.state.SetCommitIndex(n.leaderState.CommitIndex())
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
		if nextIndex < n.state.LastLogIndex() && !r.Sent() {
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
			r.SetSent()
		}
	}
}
