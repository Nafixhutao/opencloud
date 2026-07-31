package service

import (
	"context"

	"github.com/nazxf/opencloud/backend/internal/repository"
)

type domainTLSRepository interface {
	HostnameAuthorizedForTLS(context.Context, string, string) (bool, error)
}

// DomainPermissionService owns the policy used by Caddy's internal On-Demand
// TLS permission endpoint. It returns only a decision and never domain state.
type DomainPermissionService struct {
	domains          domainTLSRepository
	siteDomainSuffix string
}

// NewDomainPermissionService constructs the fail-closed permission policy.
func NewDomainPermissionService(
	domains *repository.DomainRepo,
	siteDomainSuffix string,
) *DomainPermissionService {
	return &DomainPermissionService{
		domains:          domains,
		siteDomainSuffix: siteDomainSuffix,
	}
}

// AuthorizeTLS allows only canonical public hostnames currently authorized by
// the repository's active-site and verified-domain policy.
func (s *DomainPermissionService) AuthorizeTLS(ctx context.Context, hostname string) (bool, error) {
	normalized, err := normalizeCustomHostname(hostname)
	if err != nil {
		return false, nil
	}
	return s.domains.HostnameAuthorizedForTLS(ctx, normalized, s.siteDomainSuffix)
}
