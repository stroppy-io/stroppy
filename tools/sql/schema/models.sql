CREATE TABLE IF NOT EXISTS "users" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	email TEXT  UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	refresh_tokens TEXT[]  NULL
);
CREATE TABLE IF NOT EXISTS "stroppy_steps" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
	step_run_info JSONB  NOT NULL
);
CREATE TABLE IF NOT EXISTS "resources" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	ref JSONB  NOT NULL,
	resource_def JSONB  NOT NULL,
	resource_yaml TEXT  NOT NULL,
	synced BOOLEAN  NOT NULL DEFAULT FALSE,
	ready BOOLEAN  NOT NULL DEFAULT FALSE,
	external_id TEXT  NULL DEFAULT null,
	parent_resource_id TEXT  NULL
);
CREATE TABLE IF NOT EXISTS "runs" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	status INTEGER  NOT NULL DEFAULT 0,
	target_vm_resource_id TEXT  NOT NULL,
	runner_vm_resource_id TEXT  NOT NULL,
	run_info JSONB  NOT NULL,
	grafana_dashboard_url TEXT  NULL DEFAULT null
);
