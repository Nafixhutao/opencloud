CREATE TABLE storage_objects (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id    UUID NOT NULL REFERENCES accounts(id),
  project_id    UUID NOT NULL REFERENCES projects(id),
  bucket_id     UUID NOT NULL REFERENCES storage_buckets(id),
  object_key    TEXT NOT NULL,
  size          BIGINT NOT NULL DEFAULT 0,
  content_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
  etag          TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_storage_objects_bucket_key ON storage_objects(bucket_id, object_key) WHERE deleted_at IS NULL;
CREATE INDEX idx_storage_objects_account_bucket ON storage_objects(account_id, bucket_id);
CREATE INDEX idx_storage_objects_prefix ON storage_objects(bucket_id, object_key text_pattern_ops);
