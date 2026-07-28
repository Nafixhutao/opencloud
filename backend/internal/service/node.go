package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// NodeService owns explicit global platform-admin node operations.
type NodeService struct {
	db    *bun.DB
	nodes *repository.NodeRepo
	audit *repository.AuditRepo
}

// NewNodeService constructs a NodeService.
func NewNodeService(db *bun.DB, nodes *repository.NodeRepo, audit *repository.AuditRepo) *NodeService {
	return &NodeService{db: db, nodes: nodes, audit: audit}
}

// RegisterNodeRequest is the safe operator-facing node registration payload.
type RegisterNodeRequest struct {
	Hostname      string `json:"hostname"`
	Backend       string `json:"backend"`
	CapacitySites int    `json:"capacity_sites"`
}

// Register adds schedulable capacity and audits the global operation.
func (s *NodeService) Register(ctx context.Context, actorUserID string, req RegisterNodeRequest) (*model.Node, error) {
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if hostname == "" || len(hostname) > 253 {
		return nil, apperr.Validation("invalid hostname", apperr.FieldIssue{Field: "hostname", Issue: "required, max 253"})
	}
	backend, err := provisioner.ParseBackend(req.Backend)
	if err != nil {
		return nil, apperr.Validation("invalid backend", apperr.FieldIssue{Field: "backend", Issue: "docker, hestia, or fake"})
	}
	if req.CapacitySites < 1 || req.CapacitySites > 10_000 {
		return nil, apperr.Validation(
			"invalid node capacity",
			apperr.FieldIssue{Field: "capacity_sites", Issue: "must be between 1 and 10000"},
		)
	}

	var result *model.Node
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		nodes := s.nodes.WithDB(tx)
		audit := s.audit.WithDB(tx)
		node, err := nodes.Create(ctx, hostname, string(backend), req.CapacitySites, json.RawMessage(`{}`))
		if err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			ActorID:  &actor,
			Action:   model.AuditNodeRegistered,
			Target:   strPtr(node.ID.String()),
			Metadata: map[string]any{"hostname": hostname, "backend": backend, "capacity_sites": req.CapacitySites},
		}); err != nil {
			return err
		}
		result = node
		return nil
	})
	if err != nil {
		if uniqueViolation(err) {
			return nil, apperr.Conflict("node hostname is already registered")
		}
		return nil, apperr.Internal("failed to register node").Wrap(err)
	}
	return result, nil
}

// List returns safe node capacity fields for platform admins and audits the
// global operational read before data is returned.
func (s *NodeService) List(ctx context.Context, actorUserID string) ([]model.Node, error) {
	nodes, err := s.nodes.List(ctx)
	if err != nil {
		return nil, apperr.Internal("failed to list nodes").Wrap(err)
	}
	actor := actorUserID
	if err := s.audit.Append(ctx, repository.Entry{
		ActorID:  &actor,
		Action:   model.AuditAdminNodesListed,
		Target:   strPtr("nodes"),
		Metadata: map[string]any{"result_count": len(nodes)},
	}); err != nil {
		return nil, apperr.Internal("failed to audit node list").Wrap(err)
	}
	return nodes, nil
}

// SetStatus changes admission state and audits it transactionally.
func (s *NodeService) SetStatus(ctx context.Context, actorUserID string, nodeID uuid.UUID, status string) (*model.Node, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != model.NodeOnline && status != model.NodeDraining && status != model.NodeOffline {
		return nil, apperr.Validation(
			"invalid node status",
			apperr.FieldIssue{Field: "status", Issue: "online, draining, or offline"},
		)
	}
	var result *model.Node
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		nodes := s.nodes.WithDB(tx)
		audit := s.audit.WithDB(tx)
		current, err := nodes.Get(ctx, nodeID)
		if err != nil {
			return err
		}
		if status == model.NodeOffline && current.UsedSites > 0 {
			return apperr.Conflict("drain or migrate active sites before taking a node offline")
		}
		if current.Status == status {
			result = current
			return nil
		}
		updated, err := nodes.SetStatus(ctx, nodeID, status)
		if err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			ActorID:  &actor,
			Action:   model.AuditNodeStatusChanged,
			Target:   strPtr(nodeID.String()),
			Metadata: map[string]any{"from": current.Status, "to": status},
		}); err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("node not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to update node").Wrap(err)
	}
	return result, nil
}
