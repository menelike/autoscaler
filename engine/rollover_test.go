package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	mocks_server "go.woodpecker-ci.org/autoscaler/server/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func TestHourlyRolloverUpdateFailurePreservesDrainedState(t *testing.T) {
	client := mocks_server.NewMockClient(t)
	agent := &woodpecker.Agent{
		ID: 1, Name: "pool-1-agent-paid", NoSchedule: true,
		Created:  time.Now().Add(-65 * time.Minute).Unix(),
		Platform: dockerAmd64Cap.Platform, Backend: string(dockerAmd64Cap.Backend),
	}
	a := Autoscaler{
		client: client, agents: map[string]*woodpecker.Agent{agent.Name: agent},
		providerCapabilities: []types.Capability{dockerAmd64Cap},
		config: &config.Config{
			BillingModel:               types.BillingHourlyRoundUp,
			AgentBillingTeardownMargin: 4 * time.Minute, ReconciliationInterval: time.Minute,
		},
	}
	updateErr := errors.New("update unavailable")
	client.On("AgentUpdate", mock.MatchedBy(func(updated *woodpecker.Agent) bool {
		return updated.ID == agent.ID && !updated.NoSchedule
	})).Return(nil, updateErr).Once()

	require.ErrorIs(t, a.removeDrainedAgents(t.Context()), updateErr)
	require.True(t, agent.NoSchedule)
}
