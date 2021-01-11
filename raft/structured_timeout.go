package raft

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

type ConstrainedSet interface {
	Add(*MessageWrapper)
	MarkDelivered(int)
	CheckConflict(*MessageWrapper) bool
}

type SingleConstrainedSet struct {
	N int
	// To store all the messages
	messages map[int]*MessageWrapper

	// Heartbeat timeout message IDs per replica
	heartbeats map[int]int

	// To store the timeout terms for each replica
	terms map[int]int

	// To store the dependencies for each message ID
	deps map[int][]int

	// To store the temporal dependency of the HeartBeatTimeout for a given message ID
	timeRemaining map[int]int64
	lock          *sync.Mutex
}

func NewSingleConstrainedSet(n int) *SingleConstrainedSet {
	return &SingleConstrainedSet{
		N:             n,
		messages:      make(map[int]*MessageWrapper),
		terms:         make(map[int]int),
		deps:          make(map[int][]int),
		timeRemaining: make(map[int]int64),
		heartbeats:    make(map[int]int),
		lock:          new(sync.Mutex),
	}
}

func (t *SingleConstrainedSet) Add(m *MessageWrapper) {
	if m.M.Type == "Timeout" {
		timeout := &TimeoutMessage{}
		timeout.Unmarshal([]byte(m.M.Message))

		t.lock.Lock()
		defer t.lock.Unlock()
		if _, ok := t.terms[m.From]; !ok {
			t.terms[m.From] = timeout.Term
		}
		if timeout.Term > t.terms[m.From] {
			t.terms[m.From] = timeout.Term
		}

		if timeout.T == "HeartbeatTimeout" {
			t.messages[m.ID] = m
			t.heartbeats[m.From] = m.ID
			t.timeRemaining[m.ID] = timeout.Time
		} else if timeout.T == "LeaderTimeout" {
			hid, ok := t.heartbeats[m.From]
			if !ok {
				return
			}
			if _, ok = t.messages[hid]; !ok {
				return
			}
			if t.timeRemaining[hid]-timeout.Time < 0 {
				return
			}
			t.timeRemaining[hid] = t.timeRemaining[hid] - timeout.Time
			_, ok = t.deps[hid]
			if !ok {
				t.deps[hid] = make([]int, 1)
				t.deps[hid][0] = m.ID
			} else {
				t.deps[hid] = append(t.deps[hid], m.ID)
			}
		}
	}
}

func (t *SingleConstrainedSet) MarkDelivered(id int) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.messages, id)
	delete(t.timeRemaining, id)
	delete(t.deps, id)
}

func (t *SingleConstrainedSet) CheckConflict(m *MessageWrapper) bool {
	if m.M.Type == "Timeout" {
		timeout := &TimeoutMessage{}
		timeout.Unmarshal([]byte(m.M.Message))

		t.lock.Lock()
		defer t.lock.Unlock()
		if timeout.Term < t.terms[m.From] {
			return false
		}

		if timeout.T != "HeartbeatTimeout" {
			return false
		}
		for _, d := range t.deps[m.ID] {
			if _, ok := t.messages[d]; ok {
				return true
			}
		}
		return false
	}
	return false
}

type MultipleConstrainedSet struct {
	N int
}

func NewMultipleConstrainedSet(n int) *MultipleConstrainedSet {
	return &MultipleConstrainedSet{
		N: n,
	}
}

func (t *MultipleConstrainedSet) Add(_ *MessageWrapper) {
}

func (t *MultipleConstrainedSet) MarkDelivered(_ int) {
}

func (t *MultipleConstrainedSet) CheckConflict(_ *MessageWrapper) bool {
	return false
}

type StructuredTimeoutController struct {
	out             chan *MessageWrapper
	constrainedSets []ConstrainedSet
	lock            *sync.Mutex
	counter         int
}

func NewStructuredTimeoutController(c *Config) *StructuredTimeoutController {
	return &StructuredTimeoutController{
		counter:         0,
		out:             make(chan *MessageWrapper, 100),
		constrainedSets: []ConstrainedSet{NewSingleConstrainedSet(len(c.Peers))},
		lock:            new(sync.Mutex),
	}
}

func (s *StructuredTimeoutController) NotifyMessage(m *MessageWrapper) {
	s.lock.Lock()
	m.ID = s.counter
	s.counter = s.counter + 1
	s.lock.Unlock()

	b, _ := json.Marshal(m)
	log.Printf("Received: %s\n", b)

	if m.M.Type != "Timeout" && m.M.Type != "RequestVoteReq" {
		log.Printf("Sending: %s\n", b)
		s.out <- m
		return
	}

	for _, c := range s.constrainedSets {
		c.Add(m)
	}
	go s.waitRandom(m)
}

func (s *StructuredTimeoutController) waitRandom(t *MessageWrapper) {
	duration := time.Duration(randIntn(2000)) * time.Millisecond
	time.Sleep(duration)
	flag := false
	for _, c := range s.constrainedSets {
		if c.CheckConflict(t) {
			flag = true
			break
		}
	}
	if flag {
		go s.waitRandom(t)
		return
	}
	for _, c := range s.constrainedSets {
		c.MarkDelivered(t.ID)
	}

	b, _ := json.Marshal(t)
	log.Printf("Sending: %s\n", b)
	s.out <- t
}

func (s *StructuredTimeoutController) ReceiveChan() chan *MessageWrapper {
	return s.out
}

func (s *StructuredTimeoutController) Run() {}
