package main

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
	currentTerm    uint
	raftState      RaftState
	commitIndex    uint
	lastLogApplied uint
}

func NewState(_ *Config) *State {
	return &State{
		currentTerm:    0,
		raftState:      Follower,
		commitIndex:    0,
		lastLogApplied: 0,
	}
}
