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

	leaderState     *LeaderState
	leaderStateLock *sync.Mutex

	candidateState     *CandidateState
	candidateStateLock *sync.Mutex
}

func NewNode(c *Config) *Node {
	trans := NewHTTPTransport(c)
	timer := NewStandardTimer(c)
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
		N:              len(c.Peers),
		lock:           new(sync.Mutex),
		appendLock:     new(sync.Mutex),

		leaderState:        nil,
		leaderStateLock:    new(sync.Mutex),
		candidateState:     nil,
		candidateStateLock: new(sync.Mutex),
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
}

func (n *Node) electionTimeout() {

}

func (n *Node) heartbeat() {
	lastContact := n.getLastContact()

	if time.Now().Sub(lastContact) > n.config.HeartbeatTimeout {
		// Become candidate
	}
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
	peer := n.peerStore.GetPeer(a.leaderId)
	if a.term < term {
		rep := &AppendEntriesReply{
			term:    term,
			success: false,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	if a.term > term && n.state.RaftState() != Follower {
		n.becomeFollower(a.term)
	}

	n.setLastContact()
	n.setLeader(a.leaderId)

	logAt := n.store.LogAt(a.prevLogIndex)
	if logAt == nil || logAt.term != a.prevLogTerm {
		rep := &AppendEntriesReply{
			term:    n.state.CurrentTerm(),
			success: false,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	newIndex := a.prevLogIndex + 1
	logAt = n.store.LogAt(newIndex)
	if logAt != nil && logAt.term != a.term {
		n.store.ClearFrom(a.prevLogIndex)
	}

	n.appendEntry(&LogEntry{
		command: &Command{
			data: []byte(a.command),
		},
		index: newIndex,
		term:  a.term,
	})

	lastLogIndex, _ := n.state.LastLog()
	idx := min(a.leaderCommit, lastLogIndex)
	n.state.SetCommitIndex(idx)
	go n.processLogs()

	rep := &AppendEntriesReply{
		term:    n.state.CurrentTerm(),
		success: true,
	}
	n.transport.ReplyAppendEntries(peer, rep)
	return
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
	peer := n.peerStore.GetPeer(r.candidateId)

	rep := &RequestVoteReply{
		term: curTerm,
		vote: false,
	}
	defer n.transport.ReplyRequestVote(peer, rep)

	if r.term < curTerm {
		return
	}
	lastVoteCand, lastVoteTerm := n.state.LastVote()
	if lastVoteTerm != -1 && lastVoteTerm == r.term {
		if lastVoteCand != -1 && lastVoteCand == r.candidateId {
			rep.vote = true
		}
		return
	}

	lastLogIndex, lastLogTerm := n.state.LastLog()
	if lastLogTerm > r.lastLogTerm {
		return
	}

	if lastLogTerm == r.lastLogTerm && lastLogIndex > r.lastLogIndex {
		return
	}

	n.state.SetLastVote(r.candidateId, r.term)
	rep.vote = true
	n.setLastContact()
	return
}

func (n *Node) handleRequestVoteReply(r *RequestVoteReply) {
	if n.state.RaftState() != Candidate {
		return
	}
	if r.term > n.state.CurrentTerm() {
		n.becomeFollower(r.term)
	}
	if r.vote {
		n.candidateState.IncVote()
	}
	if n.candidateState.Votes() > (n.N/2 + 1) {
		n.becomeLeader()
	}
}

func (n *Node) becomeLeader() {
	n.leaderStateLock.Lock()
	n.leaderState = NewLeaderState(n.N)
	n.state.SetRaftState(Leader)
	n.setLeader(n.id)
}

func (n *Node) becomeFollower(term int) {
	n.state.SetRaftState(Follower)
	n.state.SetCurrentTerm(term)

	n.candidateStateLock.Lock()
	n.candidateState = nil
	n.candidateStateLock.Unlock()

	n.leaderStateLock.Lock()
	n.leaderState = nil
	n.leaderStateLock.Unlock()
}

func (n *Node) handlerAppendEntriesReply(a *AppendEntriesReply) {

}

func (n *Node) handleClientRequest(c *ClientRequest) {
	if n.state.RaftState() != Leader {
		return
	}
}
