-- Create "chat" table
CREATE TABLE "chat" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "name" text NOT NULL,
  "chat_type" integer NOT NULL DEFAULT 0,
  "users_ids" text[] NOT NULL,
  "server_id" text NULL,
  "category_path" text NULL,
  "max_seq" bigint NOT NULL DEFAULT 0,
  PRIMARY KEY ("id")
);
-- Create "evet_log" table
CREATE TABLE "evet_log" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "actor_user_id" text NULL,
  "event_type" integer NOT NULL DEFAULT 0,
  "target_subscription" text NOT NULL,
  "entity_id" text NULL,
  "event" jsonb NOT NULL,
  PRIMARY KEY ("id")
);
-- Create "invite" table
CREATE TABLE "invite" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "invite_type" integer NOT NULL DEFAULT 0,
  "inviter_id" text NOT NULL,
  "acceptor_id" text NULL,
  "server_id" text NULL,
  "expires" timestamptz NULL,
  "accept_cnt" integer NOT NULL DEFAULT 0,
  "server_link" text NULL,
  PRIMARY KEY ("id")
);
-- Create "message" table
CREATE TABLE "message" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "chat_id" text NOT NULL,
  "chat_seq" bigint NOT NULL DEFAULT 0,
  "author_id" text NOT NULL,
  "text" text NOT NULL,
  "was_edited" boolean NOT NULL DEFAULT false,
  "attachments" jsonb[] NOT NULL,
  "reactions" jsonb[] NOT NULL,
  "read_by_ids" text[] NOT NULL,
  "parent_msg_id" text NULL,
  "thread_id" text NULL,
  "pinned" boolean NOT NULL DEFAULT false,
  "pin_expires" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create "role" table
CREATE TABLE "role" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "name" text NOT NULL,
  "server_id" text NOT NULL,
  "is_system" boolean NOT NULL DEFAULT false,
  "permissions" integer[] NOT NULL DEFAULT '{}',
  "users_ids" text[] NOT NULL,
  "ui_color" text NOT NULL,
  PRIMARY KEY ("id")
);
-- Create "server" table
CREATE TABLE "server" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "name" text NOT NULL,
  "server_type" integer NOT NULL DEFAULT 0,
  "visibility" integer NOT NULL DEFAULT 0,
  "avatar_id" text NULL,
  "description" text NULL,
  "owner_id" text NOT NULL,
  "members_ids" text[] NOT NULL,
  "banned_users_ids" text[] NOT NULL,
  "chat_ids" text[] NOT NULL,
  "invites_ids" text[] NOT NULL,
  "limits" jsonb NOT NULL DEFAULT '{}',
  "calendar" jsonb NOT NULL DEFAULT '{}',
  PRIMARY KEY ("id")
);
-- Create "users" table
CREATE TABLE "users" (
  "id" text NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "name" text NOT NULL,
  "avatar_id" text NULL,
  "allow_server_invites" boolean NOT NULL DEFAULT false,
  "allow_direct_chat" boolean NOT NULL DEFAULT false,
  "allow_direct_invites" boolean NOT NULL DEFAULT false,
  "online_status" integer NOT NULL DEFAULT 0,
  "last_online_at" timestamptz NOT NULL,
  "oidc_id" text NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "users_name_key" UNIQUE ("name"),
  CONSTRAINT "users_oidc_id_key" UNIQUE ("oidc_id")
);
