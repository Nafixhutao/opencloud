package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const (
	defaultSiteInternalPort = 8080
	defaultSiteMemoryBytes  = 256 * 1024 * 1024
	defaultSiteNanoCPUs     = 500_000_000
)

// SiteService owns tenant-scoped site lifecycle transitions.
type SiteService struct {
	db               *bun.DB
	sites            *repository.SiteRepo
	nodes            *repository.NodeRepo
	jobs             *repository.JobRepo
	audit            *repository.AuditRepo
	backend          string
	image            string
	siteDomainSuffix string
}

// NewSiteService constructs a SiteService.
func NewSiteService(
	db *bun.DB,
	sites *repository.SiteRepo,
	nodes *repository.NodeRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	backend string,
	image string,
	siteDomainSuffix string,
) *SiteService {
	return &SiteService{
		db:               db,
		sites:            sites,
		nodes:            nodes,
		jobs:             jobs,
		audit:            audit,
		backend:          backend,
		image:            image,
		siteDomainSuffix: strings.ToLower(strings.TrimSpace(siteDomainSuffix)),
	}
}

// CreateSiteRequest is the customer-facing create payload. Templates are
// allowlisted; arbitrary images and Dockerfiles are deliberately not accepted.
type CreateSiteRequest struct {
	Domain   string `json:"domain"`
	Template string `json:"template"`
}

type siteJobPayload struct {
	SiteID uuid.UUID `json:"site_id"`
}

// Create atomically reserves capacity, creates a site, enqueues provisioning,
// and appends its audit event.
func (s *SiteService) Create(
	ctx context.Context,
	actorUserID string,
	accountID uuid.UUID,
	idempotencyKey string,
	req CreateSiteRequest,
) (*model.Site, error) {
	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	if err := validatePrimarySiteDomain(domain, s.siteDomainSuffix); err != nil {
		return nil, err
	}
	template := strings.ToLower(strings.TrimSpace(req.Template))
	if template == "" {
		template = "static"
	}
	if template != "static" {
		return nil, apperr.Validation(
			"unsupported site template",
			apperr.FieldIssue{Field: "template", Issue: "must be static"},
		)
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 128 {
		return nil, apperr.Validation(
			"idempotency key is too long",
			apperr.FieldIssue{Field: "Idempotency-Key", Issue: "max 128"},
		)
	}

	var created *model.Site
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		sites := s.sites.WithDB(tx)
		nodes := s.nodes.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		if idempotencyKey != "" {
			if err := sites.LockCreateRequest(ctx, accountID, idempotencyKey); err != nil {
				return err
			}
			prior, priorErr := sites.GetByIdempotencyKey(ctx, accountID, idempotencyKey)
			if priorErr == nil {
				if prior.Domain != domain {
					return apperr.Conflict("idempotency key was already used for another site")
				}
				created = prior
				return nil
			}
			if !errors.Is(priorErr, sql.ErrNoRows) {
				return priorErr
			}
		}

		node, err := nodes.ClaimLeastLoaded(ctx, s.backend)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.Conflict("no hosting capacity is currently available")
			}
			return err
		}

		site := &model.Site{
			ID:             uuid.New(),
			AccountID:      accountID,
			NodeID:         node.ID,
			Domain:         domain,
			Image:          s.image,
			InternalPort:   defaultSiteInternalPort,
			MemoryBytes:    defaultSiteMemoryBytes,
			NanoCPUs:       defaultSiteNanoCPUs,
			Status:         model.SiteProvisioning,
			IdempotencyKey: optionalString(idempotencyKey),
		}
		if err := sites.Create(ctx, site); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(ctx, &aid, model.JobProvisionSite, siteJobPayload{SiteID: site.ID}); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditSiteCreateQueued,
			Target:    strPtr(site.ID.String()),
			Metadata: map[string]any{
				"domain":   domain,
				"node_id":  node.ID.String(),
				"template": template,
			},
		}); err != nil {
			return err
		}
		created = site
		return nil
	})
	if err != nil {
		if apperr.As(err) != nil {
			return nil, err
		}
		if uniqueViolation(err) {
			return nil, apperr.Conflict("domain or idempotency key is already in use")
		}
		return nil, apperr.Internal("failed to queue site creation").Wrap(err)
	}
	return created, nil
}

// List returns a paginated tenant-scoped site collection.
func (s *SiteService) List(ctx context.Context, accountID uuid.UUID, page, perPage int) ([]model.Site, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	rows, total, err := s.sites.ListByAccount(ctx, accountID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list sites").Wrap(err)
	}
	return rows, total, nil
}

// Get returns one site without revealing cross-tenant existence.
func (s *SiteService) Get(ctx context.Context, accountID, siteID uuid.UUID) (*model.Site, error) {
	site, err := s.sites.GetByAccount(ctx, accountID, siteID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("site not found")
		}
		return nil, apperr.Internal("failed to load site").Wrap(err)
	}
	return site, nil
}

// Suspend atomically records a pending transition and its durable job/audit.
func (s *SiteService) Suspend(ctx context.Context, actorUserID string, accountID, siteID uuid.UUID) (*model.Site, error) {
	return s.queueTransition(ctx, actorUserID, accountID, siteID, transition{
		from:          []string{model.SiteActive},
		pending:       model.SiteSuspending,
		idempotent:    []string{model.SiteSuspending, model.SiteSuspended},
		jobKind:       model.JobSuspendSite,
		auditAction:   model.AuditSiteSuspendQueued,
		conflictError: "site must be active before it can be suspended",
	})
}

// Resume atomically records a pending transition and its durable job/audit.
func (s *SiteService) Resume(ctx context.Context, actorUserID string, accountID, siteID uuid.UUID) (*model.Site, error) {
	return s.queueTransition(ctx, actorUserID, accountID, siteID, transition{
		from:          []string{model.SiteSuspended},
		pending:       model.SiteResuming,
		idempotent:    []string{model.SiteResuming, model.SiteActive},
		jobKind:       model.JobResumeSite,
		auditAction:   model.AuditSiteResumeQueued,
		conflictError: "site must be suspended before it can be resumed",
	})
}

// Delete queues idempotent backend cleanup and keeps the row pollable.
func (s *SiteService) Delete(ctx context.Context, actorUserID string, accountID, siteID uuid.UUID) (*model.Site, error) {
	return s.queueTransition(ctx, actorUserID, accountID, siteID, transition{
		from: []string{
			model.SiteProvisioning,
			model.SiteActive,
			model.SiteSuspending,
			model.SiteSuspended,
			model.SiteResuming,
			model.SiteFailed,
		},
		pending:        model.SiteDeleting,
		idempotent:     []string{model.SiteDeleting, model.SiteDeleted},
		jobKind:        model.JobDeleteSite,
		auditAction:    model.AuditSiteDeleteQueued,
		conflictError:  "site cannot be deleted from its current state",
		includeDeleted: true,
	})
}

type transition struct {
	from           []string
	pending        string
	idempotent     []string
	jobKind        string
	auditAction    string
	conflictError  string
	includeDeleted bool
}

func (s *SiteService) queueTransition(
	ctx context.Context,
	actorUserID string,
	accountID, siteID uuid.UUID,
	tr transition,
) (*model.Site, error) {
	var result *model.Site
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		sites := s.sites.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		if err := sites.LockRoutingTransition(ctx, siteID); err != nil {
			return err
		}
		var site *model.Site
		var err error
		if tr.includeDeleted {
			site, err = sites.GetByAccountForUpdateIncludingDeleted(ctx, accountID, siteID)
		} else {
			site, err = sites.GetByAccountForUpdate(ctx, accountID, siteID)
		}
		if err != nil {
			return err
		}
		if contains(tr.idempotent, site.Status) {
			result = site
			return nil
		}
		if !contains(tr.from, site.Status) {
			return apperr.Conflict(tr.conflictError)
		}
		if err := sites.SetStatus(ctx, accountID, siteID, tr.pending); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(ctx, &aid, tr.jobKind, siteJobPayload{SiteID: siteID}); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    tr.auditAction,
			Target:    strPtr(siteID.String()),
			Metadata:  map[string]any{"domain": site.Domain, "from": site.Status, "to": tr.pending},
		}); err != nil {
			return err
		}
		site.Status = tr.pending
		site.LastError = nil
		result = site
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("site not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to queue site lifecycle change").Wrap(err)
	}
	return result, nil
}

func normalizeDomain(value string) (string, error) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if len(domain) < 3 || len(domain) > 253 || !strings.Contains(domain, ".") {
		return "", apperr.Validation(
			"invalid domain",
			apperr.FieldIssue{Field: "domain", Issue: "must be a fully-qualified ASCII domain"},
		)
	}
	for _, r := range domain {
		if r > unicode.MaxASCII {
			return "", apperr.Validation(
				"invalid domain",
				apperr.FieldIssue{Field: "domain", Issue: "international domains must be supplied as punycode"},
			)
		}
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 ||
			label[0] == '-' || label[len(label)-1] == '-' {
			return "", apperr.Validation("invalid domain", apperr.FieldIssue{Field: "domain", Issue: "invalid DNS label"})
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return "", apperr.Validation("invalid domain", apperr.FieldIssue{Field: "domain", Issue: "invalid DNS character"})
			}
		}
	}
	return domain, nil
}

func validatePrimarySiteDomain(domain, suffix string) error {
	if suffix == "" {
		return nil
	}
	if domain == suffix || !strings.HasSuffix(domain, "."+suffix) {
		return apperr.Validation(
			"invalid platform site domain",
			apperr.FieldIssue{
				Field: "domain",
				Issue: "must be a hostname below " + suffix,
			},
		)
	}
	return nil
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
