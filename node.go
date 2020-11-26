package main

import (
	"sync"
	"time"
)

type Node struct {
	state          *State
	transport      Transport
	peerStore      *PeerStore
	id             int
	fsm            FSM
	store          Store
	config         *Config
	incommingMsgCh <-chan Message
	timer          Timers
	timeoutChan    <-chan *Timeout
	clientRequests chan *ClientRequest
	N              int
	lock           *sync.Mutex
	lastContact    time.Time
	leaderId       int
	appendLock     *sync.Mutex

	leaderState    *LeaderState
	candidateState *CandidateState
}

func NewNode(c *Config) *Node {
	trans := NewHTTPTransport(c)
	timer := NewStandardTimer(c)
	N := len(c.Peers)
	return &Node{
		state:          NewState(c),
		transport:      trans,
		incommingMsgCh: trans.ReceiveChan(),
		store:          NewMemStore(c),
		fsm:            NewKeyValueFSM(),
		config:         c,
		id:             c.CurNodeIndex,
		peerStore:      NewPeerStore(c),
		timer:          timer,
		timeoutChan:    timer.TimerChan(),
		N:              N,
		lock:           new(sync.Mutex),
		appendLock:     new(sync.Mutex),

		leaderState:    NewLeaderState(N),
		candidateState: NewCandidateState(),
	}
}

func (n *Node) Run() {
	go n.transport.Run()
	go n.timer.Run()

	for {
		select {
		case m := <-n.incommingMsgCh:
			switch m.Type() {
			case "AppendEntriesReq":
				go n.handleAppendEntries(m.(*AppendEntriesReq))
			case "RequestVoteReq":
				go n.handleRequestVote(m.(*RequestVoteReq))
			case "AppendEntriesReply":
				go n.handlerAppendEntriesReply(m.(*AppendEntriesReply))
			case "RequestVoteReply":
				go n.handleRequestVoteReply(m.(*RequestVoteReply))
			case "ClientRequest":
				go n.handleClientRequest(m.(*ClientRequest))
			}
			break
		case t := <-n.timeoutChan:
			switch t.Type {
			case "HeartbeatTimeout":
				go n.heartbeat()
			case "ElectionTimeout":
				go n.electionTimeout()
			case "LeaderTimeout":
				go n.leaderTimeout()
			}
		}
	}
}

func (n *Node) leaderTimeout() {
	if n.state.RaftState() != Leader {
		return
	}
	req := &AppendEntriesReq{
		Term:     n.state.CurrentTerm(),
		LeaderId: n.id,
	}

	for id, p := range n.peerStore.AllPeers() {
		if id != n.id {
			go n.transport.SendAppendEntries(p, req)
		}
	}
}

func (n *Node) electionTimeout() {
	if n.state.RaftState() != Candidate {
		return
	}
	n.becomeCandidate()
}

func (n *Node) heartbeat() {
	if n.state.RaftState() != Follower {
		return
	}
	lastContact := n.getLastContact()

	if time.Now().Sub(lastContact) < n.config.HeartbeatTimeout {
		return
	}
	n.becomeCandidate()
}

func (n *Node) getLastContact() time.Time {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.lastContact
}

func (n *Node) setLastContact() {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.lastContact = time.Now()
}

func (n *Node) setLeader(leaderId int) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.leaderId = leaderId
}

func (n *Node) getLeader() int {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.leaderId
}

func (n *Node) handleAppendEntries(a *AppendEntriesReq) {
	term := n.state.CurrentTerm()
	peer := n.peerStore.GetPeer(a.LeaderId)

	if a.Term < term {
		rep := &AppendEntriesReply{
			Term:    term,
			Success: false,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	if a.Term > term && n.state.RaftState() != Follower {
		n.becomeFollower(a.Term)
	}

	n.setLastContact()
	n.setLeader(a.LeaderId)

	if a.Command == "" {
		rep := &AppendEntriesReply{
			Term:    n.state.CurrentTerm(),
			Success: true,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	logAt := n.store.LogAt(a.PrevLogIndex)
	if logAt == nil || logAt.term != a.PrevLogTerm {
		rep := &AppendEntriesReply{
			Term:    n.state.CurrentTerm(),
			Success: false,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	newIndex := a.PrevLogIndex + 1
	logAt = n.store.LogAt(newIndex)
	if logAt != nil && logAt.term != a.Term {
		n.store.ClearFrom(a.PrevLogIndex)
	}

	n.appendEntry(&LogEntry{
		command: &Command{
			data: []byte(a.Command),
		},
		index: newIndex,
		term:  a.Term,
	})

	lastLogIndex, _ := n.state.LastLog()
	idx := min(a.LeaderCommit, lastLogIndex)
	n.state.SetCommitIndex(idx)
	go n.processLogs()

	rep := &AppendEntriesReply{
		Term:    n.state.CurrentTerm(),
		Success: true,
	}
	n.transport.ReplyAppendEntries(peer, rep)
	return
}

func (n *Node) handlerAppendEntriesReply(a *AppendEntriesReply) {
	if n.state.RaftState() != Leader {
		return
	}
	// Leader should interpret reply
}

func (n *Node) processLogs() {
	// Need to apply the logs on the FSM
}

func (n *Node) appendEntry(l *LogEntry) {
	n.appendLock.Lock()
	defer n.appendLock.Unlock()
	n.store.AppendLog(l)
	n.state.UpdateLastLog(l)
}

func (n *Node) handleRequestVote(r *RequestVoteReq) {
	curTerm := n.state.CurrentTerm()
	peer := n.peerStore.GetPeer(r.CandidateId)

	rep := &RequestVoteReply{
		Term: curTerm,
		Vote: false,
	}
	defer n.transport.ReplyRequestVote(peer, rep)

	if r.Term < curTerm {
		return
	}
	lastVoteCand, lastVoteTerm := n.state.LastVote()
	if lastVoteTerm != -1 && lastVoteTerm == r.Term {
		if lastVoteCand != -1 && lastVoteCand == r.CandidateId {
			rep.Vote = true
		}
		return
	}

	lastLogIndex, lastLogTerm := n.state.LastLog()
	if lastLogTerm > r.LastLogTerm {
		return
	}

	if lastLogTerm == r.LastLogTerm && lastLogIndex > r.LastLogIndex {
		return
	}

	n.state.SetLastVote(r.CandidateId, r.Term)
	rep.Vote = true
	n.setLastContact()
	return
}

func (n *Node) handleRequestVoteReply(r *RequestVoteReply) {
	if n.state.RaftState() != Candidate {
		return
	}
	if r.Term > n.state.CurrentTerm() {
		n.becomeFollower(r.Term)
	}
	if r.Vote {
		n.candidateState.IncVote()
	}
	if n.candidateState.Votes() > (n.N/2 + 1) {
		n.becomeLeader()
	}
}

func (n *Node) becomeCandidate() {
	n.candidateState.Reset()

	n.state.SetRaftState(Candidate)
	n.state.IncCurrentTerm()

	lastIndex, lastTerm := n.state.LastLog()
	req := &RequestVoteReq{
		Term:         n.state.CurrentTerm(),
		CandidateId:  n.id,
		LastLogIndex: lastIndex,
		LastLogTerm:  lastTerm,
	}

	for id, p := range n.peerStore.AllPeers() {
		if id != n.id {
			go n.transport.SendRequestVote(p, req)
		}
	}
	n.timer.StartElectionTimer()
}

func (n *Node) becomeLeader() {
	n.leaderState.Reset()

	n.state.SetRaftState(Leader)
	n.setLeader(n.id)
}

func (n *Node) becomeFollower(term int) {
	n.state.SetRaftState(Follower)
	n.state.SetCurrentTerm(term)

	n.candidateState.Reset()
	n.leaderState.Reset()
}

func (n *Node) handleClientRequest(c *ClientRequest) {
	if n.state.RaftState() != Leader {
		return
	}
	// Append to log stream and process it
}
