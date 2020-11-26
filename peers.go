package main

type Peer struct {
	addr string
	id   int
}

type PeerStore struct {
	curId int
	peers map[int]*Peer
}

func (s *PeerStore) GetPeer(id int) *Peer {
	return s.peers[id]
}

func (s *PeerStore) AllPeers() map[int]*Peer {
	return s.peers
}

func NewPeerStore(c *Config) *PeerStore {
	s := &PeerStore{
		curId: 0,
		peers: make(map[int]*Peer),
	}
	for _, p := range c.Peers {
		if s.curId != c.CurNodeIndex {
			s.peers[s.curId] = &Peer{
				addr: p,
				id:   s.curId,
			}
		}
		s.curId = s.curId + 1
	}
	return s
}
