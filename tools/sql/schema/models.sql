CREATE TABLE IF NOT EXISTS "users" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	email TEXT  UNIQUE NOT NULL,
	admin BOOLEAN  NOT NULL DEFAULT FALSE,
	password_hash TEXT NOT NULL,
	refresh_tokens TEXT[]  NULL
);
CREATE TABLE IF NOT EXISTS "run_records" (
	id TEXT  PRIMARY KEY NOT NULL,
	author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	status INTEGER  NOT NULL DEFAULT 0,
	tps JSONB  NOT NULL,
	database JSONB  NOT NULL,
	workload JSONB  NOT NULL,
	cloud_automation_id TEXT  NULL
);
CREATE TABLE IF NOT EXISTS "cloud_resources" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	status INTEGER  NOT NULL DEFAULT 0,
	ref JSONB  NOT NULL,
	resource_def JSONB  NOT NULL,
	resource_yaml TEXT  NOT NULL,
	synced BOOLEAN  NOT NULL DEFAULT FALSE,
	ready BOOLEAN  NOT NULL DEFAULT FALSE,
	external_id TEXT  NOT NULL,
	parent_resource_id TEXT  NULL
);
CREATE TABLE IF NOT EXISTS "cloud_automations" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	status INTEGER  NOT NULL DEFAULT 0,
	author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	database_root_resource_id TEXT  NOT NULL,
	workload_root_resource_id TEXT  NOT NULL,
	stroppy_run_id TEXT  NULL
);
CREATE TABLE IF NOT EXISTS "stroppy_runs" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	status INTEGER  NOT NULL DEFAULT 0,
	run_info JSONB  NOT NULL,
	grafana_dashboard_url TEXT  NOT NULL
);
CREATE TABLE IF NOT EXISTS "stroppy_steps" (
	id TEXT  PRIMARY KEY NOT NULL,
	created_at TIMESTAMPTZ  NOT NULL,
	updated_at TIMESTAMPTZ  NOT NULL,
	deleted_at TIMESTAMPTZ  NULL DEFAULT null,
	run_id TEXT NOT NULL REFERENCES stroppy_runs(id) ON DELETE CASCADE,
	step_info JSONB  NOT NULL
);
