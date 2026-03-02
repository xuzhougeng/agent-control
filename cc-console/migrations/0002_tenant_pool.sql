CREATE TABLE IF NOT EXISTS tenant_pool (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id TEXT NOT NULL,
  tenant_token TEXT NOT NULL,
  assigned_to TEXT REFERENCES users(id),
  created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pool_unassigned ON tenant_pool(assigned_to) WHERE assigned_to IS NULL;
