package agentmanager

import (
	"context"

	"github.com/cloud-print/server/internal/domain"
	"github.com/cloud-print/server/internal/storage"
)

type Repo struct {
	agentRepo *storage.AgentRepo
}

func NewRepo(agentRepo *storage.AgentRepo) *Repo {
	return &Repo{agentRepo: agentRepo}
}

func (r *Repo) CreateAgent(ctx context.Context, a *domain.Agent) error {
	return r.agentRepo.Create(ctx, a)
}

func (r *Repo) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	return r.agentRepo.Update(ctx, a)
}

func (r *Repo) GetAgent(ctx context.Context, agentID string) (*domain.Agent, error) {
	return r.agentRepo.GetByID(ctx, agentID)
}

func (r *Repo) ListAgents(ctx context.Context) ([]*domain.Agent, error) {
	return r.agentRepo.List(ctx)
}

func (r *Repo) DeleteAgent(ctx context.Context, agentID string) error {
	return r.agentRepo.Delete(ctx, agentID)
}

func (r *Repo) SetOnline(ctx context.Context, agentID string, online bool) error {
	return r.agentRepo.SetOnline(ctx, agentID, online)
}

func (r *Repo) UpdateHeartbeat(ctx context.Context, agentID, version string, onlineDevices, pendingTasks int, netClass domain.NetClass) error {
	return r.agentRepo.UpdateHeartbeat(ctx, agentID, version, onlineDevices, pendingTasks, netClass)
}

func (r *Repo) UpdateVersion(ctx context.Context, agentID, version string) error {
	return r.agentRepo.UpdateVersion(ctx, agentID, version)
}

func (r *Repo) UpdateNetClass(ctx context.Context, agentID string, netClass domain.NetClass) error {
	return r.agentRepo.UpdateNetClass(ctx, agentID, netClass)
}