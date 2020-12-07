package raft

import (
	"encoding/json"
	"time"
)

type Config struct {
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
	ElectionTimeout  time.Duration `json:"election_timeout"`
	Peers            []string      `json:"peers"`
	CurNodeIndex     int           `json:"cur_node_id"`
	MasterAddr       string        `json:"master_addr"`
	Slave            bool          `json:"slave"`

	PCTWorkingDir string `json:"pct_working_dir`
}

func ConfigFromJson(s []byte) *Config {
	c := &Config{}
	if err := json.Unmarshal(s, c); err != nil {
		panic("Could not decode config")
	}
	c.ElectionTimeout = c.ElectionTimeout * time.Millisecond
	c.HeartbeatTimeout = c.HeartbeatTimeout * time.Millisecond
	return c
}
