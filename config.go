package main

import (
	"encoding/json"
	"time"
)

type Config struct {
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
	ElectionTimeout  time.Duration `json:"election_timeout"`
}

func DefaultConfig() *Config {
	return &Config{
		HeartbeatTimeout: 1000 * time.Millisecond,
		ElectionTimeout:  1000 * time.Millisecond,
	}
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
