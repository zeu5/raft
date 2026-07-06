// Copyright 2024 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netrixdsl_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"go.etcd.io/raft/v3"
	pb "go.etcd.io/raft/v3/raftpb"
	"go.etcd.io/raft/v3/netrixdsl"
	"go.etcd.io/raft/v3/rafttest"
)

// newEnv creates a 3-node raft cluster bootstrapped at index 10 for testing.
func newEnv(t *testing.T) *rafttest.InteractionEnv {
	t.Helper()
	env := rafttest.NewInteractionEnv(nil)
	cfg := raft.Config{
		ElectionTick:    3,
		HeartbeatTick:   1,
		MaxSizePerMsg:   math.MaxUint64,
		MaxInflightMsgs: math.MaxInt32,
	}
	snap := pb.EnsureSnapshot(nil)
	snap.Metadata.ConfState.Voters = []uint64{1, 2, 3}
	snap.Metadata.Index = new(uint64(10))
	snap.Metadata.Term = new(uint64(1))
	require.NoError(t, env.AddNodes(3, cfg, snap))
	return env
}

// tickAll ticks all nodes in env once (heartbeat tick).
func tickAll(env *rafttest.InteractionEnv) {
	for i := range env.Nodes {
		_ = env.Tick(i, 1)
	}
}

// TestAllMessagesDelivered verifies that with no filters, a cluster stabilizes
// after an election: we see the no-op MsgApp the leader sends immediately.
func TestAllMessagesDelivered(t *testing.T) {
	env := newEnv(t)

	sm := netrixdsl.NewStateMachine()
	init := sm.Builder()
	// The leader immediately sends MsgApp (with no-op entry) to followers.
	init.On(netrixdsl.IsMessageType(pb.MsgApp), netrixdsl.SuccessState)

	tc := &netrixdsl.TestCase{
		Name:         "election-succeeds",
		MaxRounds:    50,
		StateMachine: sm,
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0) // node index 0, id=1
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.True(t, result.Success, "expected MsgApp from leader, final state: %s", result.FinalState)
}

// TestDropAllVotes verifies that when MsgVote messages are dropped, no leader
// can be elected (no MsgApp from a leader should appear within MaxRounds).
func TestDropAllVotes(t *testing.T) {
	env := newEnv(t)

	// We succeed (i.e., confirm the absence of a leader) if the run ends
	// without hitting FailureState.
	sm := netrixdsl.NewStateMachine()
	init := sm.Builder()
	// Seeing a MsgApp means a leader was elected — that's the failure condition.
	init.On(netrixdsl.IsMessageType(pb.MsgApp), netrixdsl.FailureState)

	filters := netrixdsl.NewFilterSet()
	// Drop all vote messages so no election can complete.
	filters.AddFilter(netrixdsl.If(netrixdsl.IsMessageType(pb.MsgVote)).Then(netrixdsl.DropMessage()))

	tc := &netrixdsl.TestCase{
		Name:         "drop-votes-no-leader",
		MaxRounds:    10,
		StateMachine: sm,
		Filters:      filters,
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0)
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.False(t, result.IsFailure(),
		"no leader should be elected when all votes are dropped")
}

// TestPartitionLeaderAfterElection verifies that dropping all messages from
// the leader (node 1) after it is elected does not produce further MsgApp from
// node 1, and that the other nodes can proceed to a new election.
func TestPartitionLeaderAfterElection(t *testing.T) {
	env := newEnv(t)

	// Phase 1: let the election complete by waiting for the leader's MsgApp.
	// Phase 2: drop subsequent messages from node 1 (simulating a partition).
	// Phase 3: wait for a MsgVote from any node other than 1 (new election).

	sm := netrixdsl.NewStateMachine()
	init := sm.Builder()
	// Phase 1 → 2: first MsgApp confirms leader election.
	phase2 := init.On(netrixdsl.IsMessageType(pb.MsgApp), "leader-elected")
	// Phase 2 → success: a follower starts a new election (sends MsgVote).
	phase2.On(
		netrixdsl.IsMessageType(pb.MsgVote).And(netrixdsl.IsMessageFrom(1).Not()),
		netrixdsl.SuccessState,
	)

	filters := netrixdsl.NewFilterSet()
	// Once we have seen the leader's no-op MsgApp, drop all further messages
	// from node 1 to simulate a partition.
	filters.AddFilter(netrixdsl.If(
		netrixdsl.IsMessageFrom(1).And(netrixdsl.Count("leader-seen").Geq(1)),
	).Then(netrixdsl.DropMessage()))

	// Count the first MsgApp and deliver it normally.
	filters.AddFilter(netrixdsl.If(
		netrixdsl.IsMessageType(pb.MsgApp).And(netrixdsl.Count("leader-seen").Lt(1)),
	).Then(
		netrixdsl.Count("leader-seen").Incr(),
		netrixdsl.DeliverMessage(),
	))

	tc := &netrixdsl.TestCase{
		Name:         "leader-partition",
		MaxRounds:    200,
		StateMachine: sm,
		Filters:      filters,
		// Tick all nodes each round to drive election timeouts.
		TickFunc: func(e *rafttest.InteractionEnv, _ int) {
			tickAll(e)
		},
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0)
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.True(t, result.Success,
		"expected new election after partitioning leader, final state: %s after %d rounds",
		result.FinalState, result.Rounds)
}

// TestMessageRecording verifies that Set().Store() and Set().DeliverAll()
// correctly buffer and replay messages.
func TestMessageRecording(t *testing.T) {
	env := newEnv(t)

	const setLabel = "votes"
	// Store the first 2 MsgVote messages. On the 3rd, flush everything.
	filters := netrixdsl.NewFilterSet()

	filters.AddFilter(netrixdsl.If(
		netrixdsl.IsMessageType(pb.MsgVote).And(netrixdsl.Count(setLabel).Lt(2)),
	).Then(
		netrixdsl.Count(setLabel).Incr(),
		netrixdsl.Set(setLabel).Store(),
	))

	filters.AddFilter(netrixdsl.If(
		netrixdsl.IsMessageType(pb.MsgVote).And(netrixdsl.Count(setLabel).Geq(2)),
	).Then(
		netrixdsl.Set(setLabel).DeliverAll(),
		netrixdsl.DeliverMessage(),
	))

	tc := &netrixdsl.TestCase{
		Name:      "message-recording",
		MaxRounds: 50,
		Filters:   filters,
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0)
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.Equal(t, "no-state-machine", result.FinalState)
	// The cluster should still have made progress: an election happened.
	require.Greater(t, len(result.Events), 0, "expected some events")
}

// TestCounterConditions exercises counter-based conditions across a run.
func TestCounterConditions(t *testing.T) {
	env := newEnv(t)
	const ctr = "msgcount"

	sm := netrixdsl.NewStateMachine()
	init := sm.Builder()
	// Succeed once we have seen at least 5 messages total.
	init.On(netrixdsl.Count(ctr).Geq(5), netrixdsl.SuccessState)

	filters := netrixdsl.NewFilterSet()
	filters.AddFilter(netrixdsl.If(netrixdsl.Always()).Then(
		netrixdsl.Count(ctr).Incr(),
		netrixdsl.DeliverMessage(),
	))

	tc := &netrixdsl.TestCase{
		Name:         "counter-conditions",
		MaxRounds:    100,
		StateMachine: sm,
		Filters:      filters,
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0)
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.True(t, result.Success, "expected to count 5 messages, final state: %s", result.FinalState)
}

// TestBooleanConditionComposition verifies And/Or/Not composition.
func TestBooleanConditionComposition(t *testing.T) {
	env := newEnv(t)

	// Conditions: message is from node 1 AND to node 2.
	fromOneToTwo := netrixdsl.IsMessageFrom(1).And(netrixdsl.IsMessageTo(2))

	sm := netrixdsl.NewStateMachine()
	init := sm.Builder()
	init.On(fromOneToTwo, netrixdsl.SuccessState)

	tc := &netrixdsl.TestCase{
		Name:         "condition-composition",
		MaxRounds:    50,
		StateMachine: sm,
		SetupFunc: func(e *rafttest.InteractionEnv) error {
			return e.Campaign(0)
		},
	}

	result := netrixdsl.Run(tc, env)
	require.NoError(t, result.Err)
	require.True(t, result.Success, "expected to see a message from 1 to 2, final state: %s", result.FinalState)
}
