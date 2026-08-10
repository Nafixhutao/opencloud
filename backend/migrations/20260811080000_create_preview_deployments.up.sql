CREATE TABLE preview_deployments (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id    UUID NOT NULL REFERENCES accounts(id),
  project_id    UUID NOT NULL REFERENCES projects(id),
  service_id    UUID NOT NULL REFERENCES services(id),
  branch        TEXT NOT NULL,
  commit_sha    TEXT NOT NULL,
  domain        TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'building',
  site_id       UUID REFERENCES sites(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at    TIMESTAMPTZ
);
CREATE INDEX idx_preview_deployments_service ON preview_deployments(service_id, branch) WHERE deleted_at IS NULL;
