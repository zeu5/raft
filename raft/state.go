package raft

import (
	"log"
	"sync"
)

type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
)

func (r RaftState) String() string {
	switch r {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return ""
	}
}

type State struct {
	lock           *sync.Mutex
	currentTerm    int
	raftState      RaftState
	commitIndex    int
	lastLogApplied int

	lastLogIndex int
	lastLogTerm  int

	lastVoteTerm      int
	lastVoteCandidate int
}

func (s *State) RaftState() RaftState {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.raftState
}

func (s *State) SetRaftState(r RaftState) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.raftState = r
}

func (s *State) CurrentTerm() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.currentTerm
}

func (s *State) IncCurrentTerm() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.currentTerm = s.currentTerm + 1
}

func (s *State) SetCurrentTerm(t int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.currentTerm = t
}

func (s *State) LastLog() (int, int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.lastLogIndex, s.lastLogTerm
}

func (s *State) UpdateLastLog(l *LogEntry) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.lastLogIndex = l.index
	s.lastLogTerm = l.term
}

func (s *State) LastLogIndex() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.lastLogIndex
}

func (s *State) CommitIndex() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.commitIndex
}

func (s *State) SetCommitIndex(i int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if i > s.commitIndex {
		log.Printf("Updaing commit index to %d\n", i)
		s.commitIndex = i
	}
}

func (s *State) LastLogApplied() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.lastLogApplied
}

func (s *State) UpdateLastLogApplied(i int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.lastLogApplied = i
}

func (s *State) LastVote() (int, int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.lastVoteCandidate, s.lastVoteTerm
}

func (s *State) SetLastVote(candidate, term int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.lastVoteCandidate = candidate
	s.lastVoteTerm = term
}

func NewState(_ *Config) *State {
	return &State{
		lock:           new(sync.Mutex),
		currentTerm:    0,
		raftState:      Follower,
		commitIndex:    0,
		lastLogApplied: 0,

		lastLogIndex: 0,
		lastLogTerm:  0,

		lastVoteCandidate: -1,
		lastVoteTerm:      -1,
	}
}

type CandidateState struct {
	votes int
	lock  *sync.Mutex
}

func NewCandidateState() *CandidateState {
	return &CandidateState{
		votes: 1,
		lock:  new(sync.Mutex),
	}
}

func (c *CandidateState) Votes() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.votes
}

func (c *CandidateState) IncVote() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.votes = c.votes + 1
}

func (c *CandidateState) Reset() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.votes = 1
}
