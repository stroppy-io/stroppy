MY_UID=$(shell id -u)
MY_GID=$(shell id -g)
generate-diff:
	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose down
	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose up --build --abort-on-container-exit postgres-migrations atlas-diff atlas-lint
	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose down -v

LOCAL_BIN := $(CURDIR)/../../bin
sqlc:
	$(LOCAL_BIN)/sqlc generate

generate-code:
#	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose down
#	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose up -d --build postgres-migrations
#	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose up --build --abort-on-container-exit atlas-apply
#	$(LOCAL_BIN)/bobgen-psql -c bobgen.yaml
#	CURRENT_UID=$(MY_UID):$(MY_GID) docker compose down
	$(LOCAL_BIN)/sqlc generate



# slq atlas migrate

MY_UID=$(shell id -u)
MY_GID=$(shell id -g)
#.sql-migrate = $(LOCAL_BIN)/sql-migrate $1 -config=$(SQL_MIGRATE_CONFIG) -env=$(SQL_MIGRATE_ENV) $2
.sql-migrate = COMPOSE_PROFILES=migrate CURRENT_UID=$(MY_UID):$(MY_GID) ARG="$(1) $(2)" docker compose -f migrate.dev.yaml up --build --abort-on-container-exit migrate

# sql migrations
.PHONY: migrate-status
migrate-status: # Статус миграций
	$(call .sql-migrate,status)

# make migrate_new NEW_MIGRATION_NAME
.PHONY: migrate-newf
migrate-new: # Создание миграции
	$(call .sql-migrate,new,$(RUN_ARGS))

# make migrate_up MIGRATION_ID
# if MIGRATION_ID is empty -> migrate to the latest version
.PHONY: migrate-up
migrate-up: # Применение миграции
	$(call .sql-migrate,apply,$(RUN_ARGS))

# make migrate_down MIGRATION_ID
# if MIGRATION_ID is empty -> migrate to the first version
.PHONY: migrate-down
migrate-down: # Откат миграции
	$(call .sql-migrate,down,$(RUN_ARGS))

.PHONY: migrate-clear
migrate-clear: migrate-down migrate-up # Откат базы через миграцию

.PHONY: .atlas
.atlas:
	$(call .sql-migrate,$(RUN_ARGS))


