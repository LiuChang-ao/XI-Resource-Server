package registry

import (
	"sync"
	"time"
)

// AgentInfo represents an online agent
type AgentInfo struct {
	AgentID        string
	Hostname       string
	MaxConcurrency int
	Paused         bool
	RunningJobs    int
	LastHeartbeat  time.Time
	ConnectedAt    time.Time
}

// Registry tracks online agents
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

// New creates a new agent registry
func New() *Registry {
	return &Registry{
		agents: make(map[string]*AgentInfo),
	}
}

// Register adds or updates an agent
func (r *Registry) Register(agentID, hostname string, maxConcurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if agent, exists := r.agents[agentID]; exists {
		agent.Hostname = hostname
		agent.MaxConcurrency = maxConcurrency
		agent.LastHeartbeat = now
	} else {
		r.agents[agentID] = &AgentInfo{
			AgentID:        agentID,
			Hostname:       hostname,
			MaxConcurrency: maxConcurrency,
			LastHeartbeat:  now,
			ConnectedAt:    now,
		}
	}
}

// UpdateHeartbeat updates agent heartbeat and status
func (r *Registry) UpdateHeartbeat(agentID string, paused bool, runningJobs int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent, exists := r.agents[agentID]; exists {
		agent.LastHeartbeat = time.Now()
		agent.Paused = paused
		agent.RunningJobs = runningJobs
	}
}

// Unregister removes an agent
func (r *Registry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
}

// GetOnline returns all online agents
func (r *Registry) GetOnline() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		// Only return agents that have sent heartbeat within last 60 seconds
		if time.Since(agent.LastHeartbeat) < 60*time.Second {
			result = append(result, agent)
		}
	}
	return result
}

// GetAgent returns agent info by ID
func (r *Registry) GetAgent(agentID string) (*AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists || time.Since(agent.LastHeartbeat) >= 60*time.Second {
		return nil, false
	}
	return agent, true
}
