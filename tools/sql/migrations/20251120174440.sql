-- Create "kv_infos" table
CREATE TABLE "kv_infos" (
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "key" text NOT NULL,
  "info" jsonb NOT NULL,
  PRIMARY KEY ("key")
);
-- Create "quotas" table
CREATE TABLE "quotas" (
  "cloud" integer NOT NULL DEFAULT 0,
  "kind" integer NOT NULL DEFAULT 0,
  "maximum" integer NOT NULL DEFAULT 0,
  "current" integer NOT NULL DEFAULT 0,
  CONSTRAINT "quotas_cloud_kind_key" UNIQUE ("cloud", "kind")
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
  PRIMARY KEY ("id"),
  CONSTRAINT "users_email_key" UNIQUE ("email")
);
-- Create "refresh_tokens" table
CREATE TABLE "refresh_tokens" (
  "user_id" text NOT NULL,
  "token" text NOT NULL,
  CONSTRAINT "refresh_tokens_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "run_records" table
CREATE TABLE "run_records" (
  "id" text NOT NULL,
  "author_id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "status" integer NOT NULL DEFAULT 0,
  "run_info" jsonb NOT NULL,
  "tps" jsonb NOT NULL,
  "grafana_dashboard_url" text NOT NULL,
  "workflow_id" text NULL,
  "cloud_run_params" jsonb NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "run_records_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "run_record_steps" table
CREATE TABLE "run_record_steps" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "run_id" text NOT NULL,
  "status" integer NOT NULL DEFAULT 0,
  "step_info" jsonb NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "run_record_steps_run_id_fkey" FOREIGN KEY ("run_id") REFERENCES "run_records" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "templates" table
CREATE TABLE "templates" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "name" text NOT NULL,
  "author_id" text NOT NULL,
  "is_default" boolean NOT NULL DEFAULT false,
  "tags" text[] NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "templates_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "workflow_tasks" table
CREATE TABLE "workflow_tasks" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "task_type" integer NOT NULL DEFAULT 0,
  "status" integer NOT NULL DEFAULT 0,
  "workflow_id" text NOT NULL,
  "cleaned_up" boolean NOT NULL DEFAULT false,
  "on_worker" text NOT NULL,
  "retry_state" jsonb NOT NULL,
  "logs" jsonb[] NOT NULL,
  "retry_settings" jsonb NOT NULL,
  "input" jsonb NOT NULL,
  "output" jsonb NOT NULL,
  "metadata" jsonb NOT NULL,
  PRIMARY KEY ("id")
);
-- Create "workflows" table
CREATE TABLE "workflows" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create "workflow_edges" table
CREATE TABLE "workflow_edges" (
  "from_id" text NOT NULL,
  "to_id" text NOT NULL,
  "workflow_id" text NULL,
  CONSTRAINT "workflow_edges_from_id_fkey" FOREIGN KEY ("from_id") REFERENCES "workflow_tasks" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "workflow_edges_to_id_fkey" FOREIGN KEY ("to_id") REFERENCES "workflow_tasks" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "workflow_edges_workflow_id_fkey" FOREIGN KEY ("workflow_id") REFERENCES "workflows" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
