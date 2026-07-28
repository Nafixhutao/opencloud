package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

const nodePlacementLockKey int64 = 0x4f434c4f55444e4f

// NodeRepo manages operator-owned hosting capacity.
type NodeRepo struct {
	db bun.IDB
}

// NewNodeRepo constructs a NodeRepo.
func NewNodeRepo(db bun.IDB) *NodeRepo {
	return &NodeRepo{db: db}
}

// WithDB returns a copy using db.
func (r *NodeRepo) WithDB(db bun.IDB) *NodeRepo {
	return &NodeRepo{db: db}
}

// Create registers a hosting node.
func (r *NodeRepo) Create(ctx context.Context, hostname, backend string, capacity int, metadata json.RawMessage) (*model.Node, error) {
	now := time.Now().UTC()
	node := &model.Node{
		ID:               uuid.New(),
		Hostname:         hostname,
		Backend:          backend,
		Status:           model.NodeOnline,
		CapacitySites:    capacity,
		ProviderMetadata: metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if len(node.ProviderMetadata) == 0 {
		node.ProviderMetadata = json.RawMessage(`{}`)
	}
	if _, err := r.db.NewInsert().Model(node).Exec(ctx); err != nil {
		return nil, err
	}
	return node, nil
}

// List returns registered nodes for the explicit platform-admin path.
func (r *NodeRepo) List(ctx context.Context) ([]model.Node, error) {
	var nodes []model.Node
	err := r.db.NewSelect().
		Model(&nodes).
		Column("id", "hostname", "backend", "status", "capacity_sites", "used_sites", "created_at", "updated_at").
		Order("created_at ASC").
		Scan(ctx)
	return nodes, err
}

// Get returns one node or sql.ErrNoRows.
func (r *NodeRepo) Get(ctx context.Context, id uuid.UUID) (*model.Node, error) {
	node := new(model.Node)
	err := r.db.NewSelect().Model(node).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// SetStatus updates a node lifecycle status.
func (r *NodeRepo) SetStatus(ctx context.Context, id uuid.UUID, status string) (*model.Node, error) {
	node := new(model.Node)
	err := r.db.NewUpdate().
		Model(node).
		Set("status = ?", status).
		Set("updated_at = now()").
		Where("id = ?", id).
		Returning("id, hostname, backend, status, capacity_sites, used_sites, provider_metadata, created_at, updated_at").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// ClaimLeastLoaded locks and increments one eligible node. Call only inside the
// same transaction that creates the resource and its job.
func (r *NodeRepo) ClaimLeastLoaded(ctx context.Context, backend string) (*model.Node, error) {
	// Placement volume is low and correctness matters more than parallelism.
	// A transaction-scoped lock makes the load ordering current after competing
	// creates commit, avoiding false "no capacity" results from SKIP LOCKED.
	if _, err := r.db.NewRaw(`SELECT pg_advisory_xact_lock(?)`, nodePlacementLockKey).Exec(ctx); err != nil {
		return nil, err
	}
	node := new(model.Node)
	err := r.db.NewRaw(`
		WITH candidate AS (
			SELECT id
			FROM nodes
			WHERE status = ?
			  AND backend = ?
			  AND used_sites < capacity_sites
			ORDER BY
			  (used_sites::numeric / capacity_sites::numeric) ASC,
			  used_sites ASC,
			  created_at ASC
			FOR UPDATE
			LIMIT 1
		)
		UPDATE nodes AS n
		SET used_sites = n.used_sites + 1,
		    updated_at = now()
		FROM candidate
		WHERE n.id = candidate.id
		RETURNING n.id, n.hostname, n.backend, n.status, n.capacity_sites,
		          n.used_sites, n.provider_metadata, n.created_at, n.updated_at`,
		model.NodeOnline,
		backend,
	).Scan(ctx, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// ReleaseCapacity decrements used capacity once a site is permanently removed.
func (r *NodeRepo) ReleaseCapacity(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.NewUpdate().
		Model((*model.Node)(nil)).
		Set("used_sites = GREATEST(used_sites - 1, 0)").
		Set("updated_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
