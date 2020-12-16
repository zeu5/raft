package raft

import (
	"encoding/json"
	"sync"
	"time"
)

type TimeoutMessage struct {
	Time int64  `json:"time"`
	T    string `json:"type"`
	Term int    `json:"term"`
}

func (t *TimeoutMessage) Type() string {
	return "Timeout"
}

func (t *TimeoutMessage) Marshal() []byte {
	data, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return data
}

func (t *TimeoutMessage) Unmarshal(d []byte) {
	json.Unmarshal(d, t)
}

type TimeoutTransport interface {
	TimeoutRecvChan() chan *TimeoutMessage
	SendTimeout(*TimeoutMessage)
}

type ControlledTimer struct {
	transport              TimeoutTransport
	outChan                chan *Timeout
	inChan                 chan *TimeoutMessage
	heartBeat              time.Duration
	election               time.Duration
	HeartbeatTerm          int
	ElectionTerm           int
	heartbeatTermLock      *sync.Mutex
	electionTermLock       *sync.Mutex
	HeartbeatTimeout       int64
	ElectionTimeout        int64
	leaderCounter          int
	heartbeatCounter       int
	electionTimeoutCounter int
	counterLock            *sync.Mutex
}

func NewControlledTimer(c *Config, trans TimeoutTransport) *ControlledTimer {
	timer := &ControlledTimer{
		transport:              trans,
		outChan:                make(chan *Timeout, 10),
		inChan:                 trans.TimeoutRecvChan(),
		heartBeat:              c.HeartbeatTimeout,
		election:               c.ElectionTimeout,
		HeartbeatTerm:          0,
		heartbeatTermLock:      new(sync.Mutex),
		ElectionTerm:           0,
		electionTermLock:       new(sync.Mutex),
		HeartbeatTimeout:       c.HeartbeatTimeout.Milliseconds(),
		ElectionTimeout:        c.ElectionTimeout.Milliseconds(),
		leaderCounter:          0,
		heartbeatCounter:       0,
		electionTimeoutCounter: 0,
	}
	return timer
}

func (c *ControlledTimer) TimerChan() <-chan *Timeout {
	return c.outChan
}

func (c *ControlledTimer) sendHeartbeat() {
	c.heartbeatTermLock.Lock()
	term := c.HeartbeatTerm
	count := c.heartbeatCounter
	c.heartbeatCounter = c.heartbeatCounter + 1
	c.heartbeatTermLock.Unlock()

	if count > 5 {
	}

	c.transport.SendTimeout(&TimeoutMessage{
		Time: c.HeartbeatTimeout,
		T:    "HeartbeatTimeout",
		Term: term,
	})
}

func (c *ControlledTimer) sendLeaderbeat() {
	c.heartbeatTermLock.Lock()
	term := c.HeartbeatTerm
	count := c.leaderCounter
	c.leaderCounter = c.leaderCounter + 1
	c.heartbeatTermLock.Unlock()

	if count > 10 {
	}

	c.transport.SendTimeout(&TimeoutMessage{
		Time: c.HeartbeatTimeout / 10,
		T:    "LeaderTimeout",
		Term: term,
	})
}

func (c *ControlledTimer) checkHeartbeatTerm(t int) bool {
	c.heartbeatTermLock.Lock()
	defer c.heartbeatTermLock.Unlock()
	return c.HeartbeatTerm == t
}

func (c *ControlledTimer) checkElectionTerm(t int) bool {
	c.electionTermLock.Lock()
	defer c.electionTermLock.Unlock()
	return c.ElectionTerm == t
}

func (c *ControlledTimer) Run() {
	c.sendHeartbeat()
	c.sendLeaderbeat()
	for m := range c.inChan {
		switch m.T {
		case "HeartbeatTimeout":
			if c.checkHeartbeatTerm(m.Term) {
				go c.sendHeartbeat()
				c.outChan <- &Timeout{
					Type:   "HeartbeatTimeout",
					Millis: m.Time,
				}
			}
		case "ElectionTimeout":
			if c.checkElectionTerm(m.Term) {
				c.outChan <- &Timeout{
					Type:   "ElectionTimeout",
					Millis: m.Time,
				}
			}
		case "LeaderTimeout":
			if c.checkHeartbeatTerm(m.Term) {
				go c.sendLeaderbeat()
				c.outChan <- &Timeout{
					Type:   "LeaderTimeout",
					Millis: m.Time,
				}
			}
		}
	}
}

func (c *ControlledTimer) RestartHeartbeat() {
	c.heartbeatTermLock.Lock()
	c.HeartbeatTerm = c.HeartbeatTerm + 1
	c.heartbeatTermLock.Unlock()

	c.sendHeartbeat()
	c.sendLeaderbeat()
}

func (c *ControlledTimer) StartElectionTimer() {
	c.electionTermLock.Lock()
	c.ElectionTerm = c.ElectionTerm + 1
	term := c.ElectionTerm
	count := c.electionTimeoutCounter
	c.electionTimeoutCounter = c.electionTimeoutCounter + 1
	c.electionTermLock.Unlock()

	if count > 5 {
	}

	c.transport.SendTimeout(&TimeoutMessage{
		Time: c.ElectionTimeout,
		T:    "ElectionTimeout",
		Term: term,
	})
}
