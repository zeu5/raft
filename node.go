package main

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
	}
}

func (n *Node) Run() {
	go n.transport.Run()
	go n.timer.Run()

	for {
		select {
		case m := <-n.incommingMsgCh:
			switch m.Type() {
			case "AppendEntries":
				n.handleAppendEntries(m.(*AppendEntries))
			case "RequestVote":
				n.handleRequestVote(m.(*RequestVote))
			}
			break
		case t := <-n.timeoutChan:
			switch t.Type {
			case "HeartbeatTimeout":
			case "ElectionTimeout":
			case "LeaderTimeout":
			}
		}
	}
}

func (n *Node) handleAppendEntries(a *AppendEntries) {

}

func (n *Node) handleRequestVote(r *RequestVote) {

}
