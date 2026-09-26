package e2e_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// Exercise the combined scheduler and rollover changes with our two pool labels.
func TestHourlyRolloverWithMixedPools(t *testing.T) {
	t.Parallel()
	for _, pending := range []int{0, 1, 2} {
		for _, workers := range []int{0, 1} {
			t.Run(fmt.Sprintf("pending=%d/workers=%d", pending, workers), func(t *testing.T) {
				t.Parallel()
				cfg := hourlyConfig()
				cfg.MaxAgents = 2
				cfg.AgentBillingTeardownMargin = 4 * time.Minute
				cfg.ExtraAgentLabels = map[string]string{"type": "cloud"}
				h := newHarness(t, cfg, dockerAMD64)
				agent := h.addConnectedAgent(t, "pool-e2e-agent-paid", dockerAMD64)
				agent.Created = time.Now().Add(-65 * time.Minute).Unix()
				agent.NoSchedule = true
				h.woodpecker.put(agent)
				h.woodpecker.queue.Stats.Workers = workers
				// Homelab demand and global workers must not affect the cloud pool.
				homelab := realWorkflowTask("homelab", "linux/amd64")
				homelab.Labels["type"] = "homelab"
				h.woodpecker.queue.Pending = []woodpecker.Task{homelab}
				for i := range pending {
					task := realWorkflowTask(fmt.Sprintf("cloud-%d", i), "linux/amd64")
					task.Labels["type"] = "cloud"
					h.woodpecker.queue.Pending = append(h.woodpecker.queue.Pending, task)
				}

				h.reconcile(t)
				require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
				require.Len(t, h.provider.deployed, max(1, pending))
				require.Equal(t, int64(max(1, pending)+1), h.woodpecker.nextID)
				// Repeated snapshots must not create more machines while one boots.
				h.reconcile(t)
				require.Len(t, h.provider.deployed, max(1, pending))
			})
		}
	}
}

func TestHourlyRolloverProtectsRunningWork(t *testing.T) {
	t.Parallel()
	h := newHarness(t, hourlyConfig(), dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-busy", dockerAMD64)
	agent.Created = time.Now().Add(-59 * time.Minute).Unix()
	agent.NoSchedule = true
	h.woodpecker.put(agent)
	h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("busy", "linux/amd64"), agent.ID)}
	h.reconcile(t)
	require.True(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
	require.Contains(t, h.provider.deployed, agent.Name)

	agent.Created = time.Now().Add(-65 * time.Minute).Unix()
	h.woodpecker.put(agent)
	h.reconcile(t)
	require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
	require.Contains(t, h.provider.deployed, agent.Name)
	h.reconcile(t)
	require.Contains(t, h.provider.deployed, agent.Name)

	agent = h.woodpecker.agentByName(t, agent.Name)
	agent.Created = time.Now().Add(-119 * time.Minute).Unix()
	h.woodpecker.put(agent)
	h.woodpecker.queue.Running = nil
	h.reconcile(t)
	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

func TestHourlyRolloverKeepsIneligibleAgentsDrained(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		created int64
		drifted bool
	}{
		{name: "unknown creation time"},
		{name: "future creation time", created: time.Now().Add(time.Hour).Unix()},
		{name: "first paid hour", created: time.Now().Add(-5 * time.Minute).Unix()},
		{name: "outdated labels", created: time.Now().Add(-65 * time.Minute).Unix(), drifted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, hourlyConfig(), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-drained", dockerAMD64)
			agent.Created = tc.created
			agent.NoSchedule = true
			if tc.drifted {
				agent.CustomLabels = map[string]string{"type": "old"}
			}
			h.woodpecker.put(agent)
			h.reconcile(t)
			require.True(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
			require.Contains(t, h.provider.deployed, agent.Name)
		})
	}
}
