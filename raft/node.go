package raft

import (
	"context"
	"log"
	"net/http"
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
	lastContact    bool
	leaderId       int
	appendLock     *sync.Mutex
	shutdown       chan bool

	leaderState    *LeaderState
	candidateState *CandidateState
}

func NewNode(c *Config) *Node {
	peerStore := NewPeerStore(c)
	trans := NewHTTPTransport(c, peerStore)
	timer := NewControlledTimer(c, trans) //NewStandardTimer(c)
	N := len(c.Peers)
	state := NewState(c)
	return &Node{
		state:          state,
		transport:      trans,
		incommingMsgCh: trans.ReceiveChan(),
		store:          NewMemStore(c),
		fsm:            NewKeyValueFSM(),
		config:         c,
		id:             c.CurNodeIndex,
		peerStore:      peerStore,
		timer:          timer,
		timeoutChan:    timer.TimerChan(),
		N:              N,
		lock:           new(sync.Mutex),
		appendLock:     new(sync.Mutex),
		lastContact:    false,

		leaderState:    NewLeaderState(N, peerStore, state),
		candidateState: NewCandidateState(),
		shutdown:       make(chan bool),
	}
}

func (n *Node) Run(ctx context.Context) {

	nCtx, cancel := context.WithCancel(context.Background())
	stopChan := make(chan struct{}, 1)

	go func() {
		<-ctx.Done()
		cancel()
		stopChan <- struct{}{}
	}()

	go n.transport.Run(nCtx)
	go n.timer.Run()
	go n.processLogs()

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
				go n.heartbeat(t)
			case "ElectionTimeout":
				go n.electionTimeout(t)
			case "LeaderTimeout":
				go n.leaderTimeout(t)
			}
		case <-stopChan:
			break
		}
	}
}

func (n *Node) leaderTimeout(_ *Timeout) {
	if n.state.RaftState() != Leader {
		return
	}
	req := &AppendEntriesReq{
		Term:         n.state.CurrentTerm(),
		LeaderId:     n.id,
		LeaderCommit: n.state.CommitIndex(),
	}

	for id, p := range n.peerStore.AllPeers() {
		if id != n.id {
			go n.transport.SendAppendEntries(p, req)
		}
	}
}

func (n *Node) electionTimeout(_ *Timeout) {
	if n.state.RaftState() != Candidate {
		return
	}
	n.becomeCandidate()
}

func (n *Node) heartbeat(t *Timeout) {
	if n.state.RaftState() != Follower {
		return
	}
	// log.Printf("Heartbeat timeout of %d\n", t.Millis)
	lastContact := n.getLastContact()

	if lastContact {
		n.setLastContact(false)
		return
	}
	n.becomeCandidate()
}

func (n *Node) getLastContact() bool {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.lastContact
}

func (n *Node) setLastContact(v bool) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.lastContact = v
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
	lastLogIndex, _ := n.state.LastLog()
	peer := n.peerStore.GetPeer(a.LeaderId)

	if a.Term < term {
		rep := &AppendEntriesReply{
			ReplicaID: n.id,
			Term:      term,
			Success:   false,
			LastLog:   lastLogIndex,
			HeartBeat: false,
		}
		n.transport.ReplyAppendEntries(peer, rep)
		return
	}

	if a.Term > term || n.state.RaftState() != Follower {
		n.becomeFollower(a.Term)
	}

	n.setLastContact(true)
	n.setLeader(a.LeaderId)

	if a.PrevLogIndex > 0 {
		logAt := n.store.LogAt(a.PrevLogIndex)
		if logAt == nil || logAt.term != a.PrevLogTerm {
			rep := &AppendEntriesReply{
				ReplicaID: n.id,
				Term:      n.state.CurrentTerm(),
				Success:   false,
				LastLog:   lastLogIndex,
				HeartBeat: false,
			}
			n.transport.ReplyAppendEntries(peer, rep)
			return
		}
	}

	if a.Command != "" {
		newIndex := a.PrevLogIndex + 1
		logAt := n.store.LogAt(newIndex)
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
	}

	lastLogIndex, _ = n.state.LastLog()
	idx := min(a.LeaderCommit, lastLogIndex)
	n.state.SetCommitIndex(idx)

	rep := &AppendEntriesReply{
		ReplicaID: n.id,
		Term:      n.state.CurrentTerm(),
		Success:   true,
		LastLog:   lastLogIndex,
		HeartBeat: a.Command == "",
	}
	n.transport.ReplyAppendEntries(peer, rep)
	return
}

func (n *Node) handlerAppendEntriesReply(a *AppendEntriesReply) {
	if n.state.RaftState() != Leader {
		return
	}
	// Leader should interpret reply
	if a.Term > n.state.CurrentTerm() {
		n.becomeFollower(a.Term)
		n.setLastContact(true)
		return
	}
	if !a.HeartBeat {
		n.leaderState.UpdateReplicationStatus(a.ReplicaID, a.Success, a.LastLog)
	}
}

func (n *Node) processLogs() {
	// Need to apply the logs on the FSM
	for {

		lastLogApplied := n.state.LastLogApplied()
		commitIndex := n.state.CommitIndex()
		lla := lastLogApplied + 1

		if commitIndex > lastLogApplied {
			slice := n.store.Slice(lastLogApplied+1, commitIndex)
			for _, log := range slice {
				// Todo: reply to client
				n.fsm.ApplyCommand(log.command)
				n.state.UpdateLastLogApplied(lla)
				lla = lla + 1
			}
		}

		select {
		case <-n.shutdown:
			return
		default:
		}

		// Todo: Parameterize this
		time.Sleep(100 * time.Millisecond)
	}
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
		ReplicaID: n.id,
		Term:      curTerm,
		Vote:      false,
		ForTerm:   r.Term,
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
	n.timer.RestartHeartbeat()
	n.setLastContact(false)
	return
}

func (n *Node) handleRequestVoteReply(r *RequestVoteReply) {
	if n.state.RaftState() != Candidate {
		return
	}
	curTerm := n.state.CurrentTerm()
	if r.Term > curTerm {
		n.becomeFollower(r.Term)
	}
	if r.Vote && r.ForTerm == curTerm {
		// Todo: Add vote based on replica ID
		n.candidateState.IncVote()
	}
	if n.candidateState.Votes() >= (n.N/2 + 1) {
		n.becomeLeader()
	}
}

func (n *Node) becomeCandidate() {
	log.Printf("Becoming candidate\n")
	n.candidateState.Reset()

	n.state.SetRaftState(Candidate)
	n.setLeader(-1)
	n.state.IncCurrentTerm()

	lastIndex, lastTerm := n.state.LastLog()
	curTerm := n.state.CurrentTerm()
	n.state.SetLastVote(n.id, curTerm)
	req := &RequestVoteReq{
		Term:         curTerm,
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
	log.Printf("Becoming leader\n")
	n.leaderState.Reset()

	n.state.SetRaftState(Leader)
	n.setLeader(n.id)
	n.timer.RestartHeartbeat()
	go n.informMaster()
	go n.leaderLoop()
}

func (n *Node) becomeFollower(term int) {
	log.Printf("Becoming follower\n")
	n.state.SetRaftState(Follower)
	n.state.SetCurrentTerm(term)

	n.leaderState.Stop()
	n.timer.RestartHeartbeat()
}

func (n *Node) handleClientRequest(c *ClientRequest) {
	if n.state.RaftState() != Leader {
		c.resp <- &ClientResponse{
			text:       "Not leader",
			statuscode: http.StatusBadRequest,
		}
		return
	}
	n.leaderState.incoming <- &Command{data: []byte(c.Command)}
	c.resp <- &ClientResponse{
		text:       "OK",
		statuscode: http.StatusOK,
	}
}
