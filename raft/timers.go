package raft

import (
	"time"
)

type Timeout struct {
	Type string
}

type Timers interface {
	TimerChan() <-chan *Timeout
	StartElectionTimer()
	RestartHeartbeat()
	Run()
}

type StandardTimer struct {
	heartbeatTimeout time.Duration
	electionTimeout  time.Duration
	leaderTimer      <-chan time.Time
	heartbeatTimer   <-chan time.Time
	electionTimer    <-chan time.Time
	outChan          chan *Timeout
}

func NewStandardTimer(c *Config) *StandardTimer {

	t := &StandardTimer{
		heartbeatTimeout: c.HeartbeatTimeout,
		electionTimeout:  c.ElectionTimeout,
		electionTimer:    make(<-chan time.Time),
		leaderTimer:      make(<-chan time.Time),
		heartbeatTimer:   make(<-chan time.Time),
		outChan:          make(chan *Timeout, 10),
	}

	hExtra := time.Duration(time.Duration(randomInt()) % t.heartbeatTimeout)

	leaderTimeout := t.heartbeatTimeout / 10
	lExtra := time.Duration(time.Duration(randomInt()) % leaderTimeout)

	t.heartbeatTimer = time.After(t.heartbeatTimeout + hExtra)
	t.leaderTimer = time.After(leaderTimeout + lExtra)

	return t
}

func (t *StandardTimer) TimerChan() <-chan *Timeout {
	return t.outChan
}

func (t *StandardTimer) StartElectionTimer() {
	extra := time.Duration(time.Duration(randomInt()) % t.electionTimeout)
	t.electionTimer = time.After(t.electionTimeout + extra)
}

func (t *StandardTimer) RestartHeartbeat() {
	t.restartHeartbeat()
	t.restartLeaderHeartbeat()
}

func (t *StandardTimer) restartHeartbeat() {
	extra := time.Duration(time.Duration(randomInt()) % t.heartbeatTimeout)
	t.heartbeatTimer = time.After(t.heartbeatTimeout + extra)
}

func (t *StandardTimer) restartLeaderHeartbeat() {
	lTimeout := t.heartbeatTimeout / 10
	extra := time.Duration(time.Duration(randomInt()) % lTimeout)
	t.leaderTimer = time.After(lTimeout + extra)
}

func (t *StandardTimer) Run() {

	for {
		select {
		case _ = <-t.heartbeatTimer:
			t.restartHeartbeat()
			t.outChan <- &Timeout{
				Type: "HeartbeatTimeout",
			}
		case _ = <-t.electionTimer:
			t.outChan <- &Timeout{
				Type: "ElectionTimeout",
			}
		case _ = <-t.leaderTimer:
			t.restartLeaderHeartbeat()
			t.outChan <- &Timeout{
				Type: "LeaderTimeout",
			}
		default:
		}
	}
}
