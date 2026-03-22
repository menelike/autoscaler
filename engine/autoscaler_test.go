package engine

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	mocks_engine "go.woodpecker-ci.org/autoscaler/engine/mocks"
	mocks_server "go.woodpecker-ci.org/autoscaler/server/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type MockClient struct {
	workers       int
	running       int
	pending       int
	waitingOnDeps int
	woodpecker.Client
}

func (m MockClient) QueueInfo() (*woodpecker.Info, error) {
	info := &woodpecker.Info{}

	info.Stats.Workers = m.workers
	info.Stats.Running = m.running
	info.Stats.Pending = m.pending
	info.Stats.WaitingOnDeps = m.waitingOnDeps

	info.Pending = []woodpecker.Task{
		{
			Labels: map[string]string{
				"arch": "amd64",
			},
		},
	}
	info.Running = []woodpecker.Task{
		{
			Labels: map[string]string{
				"arch": "amd64",
			},
		},
	}

	return info, nil
}

func Test_calcAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("should create new agent (MinAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 0,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
			MinAgents:         1,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create single agent", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create multiple agents", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 6,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(2), value)
	})

	t.Run("should create new agent (MaxAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
		}, agents: []*woodpecker.Agent{
			{Name: "pool-1-agent-1234"},
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should not create new agent (availableAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			workers: 2,
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(0), value)
	})
}

func Test_getQueueInfo(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	t.Run("should not filter", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{
				pending: 2,
			},
			config: &config.Config{},
		}

		free, running, pending, _ := autoscaler.getQueueInfo(t.Context())
		assert.Equal(t, 0, free)
		assert.Equal(t, 0, running)
		assert.Equal(t, 2, pending)
	})

	t.Run("should filter one by label", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{
				pending: 2,
			},
			config: &config.Config{
				FilterLabels: "arch=amd64",
			},
		}

		free, running, pending, _ := autoscaler.getQueueInfo(t.Context())
		assert.Equal(t, 0, free)
		assert.Equal(t, 1, running)
		assert.Equal(t, 1, pending)
	})

	t.Run("should filter all by label", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{
				pending: 2,
			},
			config: &config.Config{
				FilterLabels: "arch=arm64",
			},
		}

		free, running, pending, _ := autoscaler.getQueueInfo(t.Context())
		assert.Equal(t, 0, free)
		assert.Equal(t, 0, running)
		assert.Equal(t, 0, pending)
	})
}

func Test_getPoolAgents(t *testing.T) {
	autoscaler := Autoscaler{
		agents: []*woodpecker.Agent{
			{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
			{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
		},
	}

	agents := autoscaler.getPoolAgents(false)
	assert.Equal(t, 3, len(agents))

	agents = autoscaler.getPoolAgents(true)
	assert.Equal(t, 2, len(agents))
}

func Test_createAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("should create a new agent", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			config: &config.Config{
				PoolID: "1",
			},
		}

		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything).Return(nil)

		err := autoscaler.createAgents(ctx, 1)
		assert.NoError(t, err)
	})

	t.Run("should reuse an no-schedule agent first before creating a new one", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			agents: []*woodpecker.Agent{
				{
					ID:         1,
					NoSchedule: true,
				},
			},
			config: &config.Config{
				PoolID: "1",
			},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 1 && agent.NoSchedule == false
		})).Return(nil, nil)
		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything).Return(nil)

		err := autoscaler.createAgents(ctx, 2)
		assert.NoError(t, err)
	})
}

func Test_cleanupDanglingAgents(t *testing.T) {
	t.Run("should remove agent that is only present on woodpecker (not provider)", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		provider.On("ListDeployedAgentNames", mock.Anything).Return(nil, nil)
		client.On("AgentDelete", int64(1)).Return(nil)

		err := autoscaler.cleanupDanglingAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should remove agent that is only present on provider (not woodpecker)", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		provider.On("ListDeployedAgentNames", mock.Anything).Return([]string{"pool-1-agent-1", "pool-1-agent-2"}, nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.Name == "pool-1-agent-2"
		})).Return(nil)

		err := autoscaler.cleanupDanglingAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_cleanupStaleAgents(t *testing.T) {
	t.Run("should remove agent that never connected (last contact = 0) in over 15 minutes", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),  // last contact 5 minutes ago
				},
				{
					ID:          2,
					Name:        "never contacted agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: 0,                                        // never contacted
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentInactivityTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		client.On("AgentDelete", int64(2)).Return(nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)

		err := autoscaler.cleanupStaleAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should remove agent that has lost connection for more than 15 minutes", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),  // last contact 5 minutes ago
				},
				{
					ID:          2,
					Name:        "stale agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 20).Unix(), // last contact 20 minutes ago
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentInactivityTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		client.On("AgentDelete", int64(2)).Return(nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)

		err := autoscaler.cleanupStaleAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_isAgentIdle(t *testing.T) {
	t.Run("should return false if agent has tasks", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return([]*woodpecker.Task{
			{ID: "1"},
		}, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
		})
		assert.NoError(t, err)
		assert.False(t, idle)
	})

	t.Run("should return false if agent has done work recently", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return(nil, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
			LastWork:   time.Now().Add(-time.Minute * 10).Unix(),
		})
		assert.NoError(t, err)
		assert.False(t, idle)
	})

	t.Run("should return true if agent is idle", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return(nil, nil) // no tasks

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
			LastWork:   time.Now().Add(-time.Minute * 20).Unix(), // last work 20 minutes ago
		})
		assert.NoError(t, err)
		assert.True(t, idle)
	})

}

func Test_isWithinBillingWindow(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("should return true when well within billing interval", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created 20 min ago — 40 min left in billing hour
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 20).Unix(),
		})
		assert.True(t, result, "should keep agent — 40 min remaining in billing hour")
	})

	t.Run("should return false near billing boundary", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created 57 min ago — 3 min left (within 5 min buffer)
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 57).Unix(),
		})
		assert.False(t, result, "should allow teardown — only 3 min left in billing hour")
	})

	t.Run("should return false when not configured", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{},
		}

		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 20).Unix(),
		})
		assert.False(t, result, "should not apply — no billing interval configured")
	})

	t.Run("should work across billing boundaries", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created 80 min ago — 80 % 60 = 20 min into 2nd cycle, 40 min remaining
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 80).Unix(),
		})
		assert.True(t, result, "should keep agent — 40 min remaining in 2nd billing hour")
	})

	t.Run("should keep agent alive when idle timeout crosses billing boundary", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created 63 min ago — job finished at min 58, idle timeout passed at min 63.
		// Now 3 min into the 2nd billing hour, 57 min remaining.
		// Should keep alive to maximize the new billing cycle.
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 63).Unix(),
		})
		assert.True(t, result, "should keep agent — 57 min remaining in 2nd billing hour")
	})

	t.Run("should respect custom buffer", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 10,
			},
		}

		// Agent created 52 min ago — 8 min left, within 10 min buffer
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 52).Unix(),
		})
		assert.False(t, result, "should allow teardown — 8 min left with 10 min buffer")
	})

	t.Run("should allow teardown when remaining equals buffer exactly", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created 55 min ago — exactly 5 min left == buffer
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Minute * 55).Unix(),
		})
		assert.False(t, result, "should allow teardown — remaining equals buffer exactly")
	})

	t.Run("should allow teardown at exact billing boundary", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent created exactly 60 min ago — 0 remaining (new cycle just started, but remaining wraps to ~60min)
		// 60 min % 60 min = 0 elapsed, remaining = 60 min — this is the start of a new cycle
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Add(-time.Hour).Unix(),
		})
		assert.True(t, result, "should keep alive — just entered new billing cycle, 60 min remaining")
	})

	t.Run("should keep alive when just created", func(t *testing.T) {
		autoscaler := Autoscaler{
			config: &config.Config{
				BillingInterval: time.Hour,
				BillingBuffer:   time.Minute * 5,
			},
		}

		// Agent just created — full hour remaining
		result := autoscaler.isWithinBillingWindow(&woodpecker.Agent{
			Name:    "pool-1-agent-1",
			Created: time.Now().Unix(),
		})
		assert.True(t, result, "should keep alive — just created, full billing hour remaining")
	})
}

func Test_drainAgents(t *testing.T) {
	t.Run("should drain agents and skip no-schedule ones", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: true, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 4, Name: "pool-1-agent-4", NoSchedule: false, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return (agent.ID == 1 || agent.ID == 4) && agent.NoSchedule == true
		})).Return(nil, nil)

		err := autoscaler.drainAgents(ctx, 2)
		assert.NoError(t, err)
		assert.True(t, autoscaler.agents[0].NoSchedule)
		assert.True(t, autoscaler.agents[3].NoSchedule)
	})

	t.Run("should not remove an agent that never connected", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false, LastContact: 0},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents[0].NoSchedule)
	})

	t.Run("should not remove an agent that has recently done some work", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "pool-1-agent-1",
					NoSchedule:  false,
					LastContact: time.Now().Add(-time.Minute * 2).Unix(), // last contact 2 minutes ago
					LastWork:    time.Now().Add(-time.Minute * 5).Unix(), // last work 5 minutes ago
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents[0].NoSchedule)
	})
}

func Test_removeDrainedAgents(t *testing.T) {
	t.Run("should remove agent", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)
		client.On("AgentDelete", int64(2)).Return(nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should not remove agent with tasks", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		client.On("AgentTasksList", int64(2)).Return([]*woodpecker.Task{
			{ID: "1"},
		}, nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})
}
