-- Create "cloud_resources" table
CREATE TABLE "cloud_resources" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "status" integer NOT NULL DEFAULT 0,
  "ref" jsonb NOT NULL,
  "resource_def" jsonb NOT NULL,
  "resource_yaml" text NOT NULL,
  "synced" boolean NOT NULL DEFAULT false,
  "ready" boolean NOT NULL DEFAULT false,
  "external_id" text NOT NULL,
  "parent_resource_id" text NULL,
  PRIMARY KEY ("id")
);
-- Create "users" table
CREATE TABLE "users" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "email" text NOT NULL,
  "admin" boolean NOT NULL DEFAULT false,
  "password_hash" text NOT NULL,
  "refresh_tokens" text[] NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "users_email_key" UNIQUE ("email")
);
-- Create "cloud_automations" table
CREATE TABLE "cloud_automations" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "status" integer NOT NULL DEFAULT 0,
  "author_id" text NOT NULL,
  "database_root_resource_id" text NOT NULL,
  "workload_root_resource_id" text NOT NULL,
  "stroppy_run_id" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "cloud_automations_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "run_records" table
CREATE TABLE "run_records" (
  "id" text NOT NULL,
  "author_id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "status" integer NOT NULL DEFAULT 0,
  "tps" jsonb NOT NULL,
  "database" jsonb NOT NULL,
  "workload" jsonb NOT NULL,
  "cloud_automation_id" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "run_records_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "stroppy_runs" table
CREATE TABLE "stroppy_runs" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "status" integer NOT NULL DEFAULT 0,
  "run_info" jsonb NOT NULL,
  "grafana_dashboard_url" text NOT NULL,
  PRIMARY KEY ("id")
);
-- Create "stroppy_steps" table
CREATE TABLE "stroppy_steps" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "run_id" text NOT NULL,
  "step_info" jsonb NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "stroppy_steps_run_id_fkey" FOREIGN KEY ("run_id") REFERENCES "stroppy_runs" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
