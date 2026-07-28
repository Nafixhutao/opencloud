package model

// ResourceOverview contains tenant-scoped dashboard aggregates. Counts exclude
// soft-deleted resources and never expose resource identifiers or provider
// details.
type ResourceOverview struct {
	SitesTotal      int `bun:"sites_total" json:"sites_total"`
	SitesActive     int `bun:"sites_active" json:"sites_active"`
	DatabasesTotal  int `bun:"databases_total" json:"databases_total"`
	DatabasesActive int `bun:"databases_active" json:"databases_active"`
}
