env "docker" {
  dev = "postgres://postgres:developer@postgres-migrations:5432/postgres?search_path=public&sslmode=disable"
  url = "postgres://postgres:developer@postgres-migrations:5432/postgres?search_path=public&sslmode=disable"
  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
    schema {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "local" {
  dev = "postgres://postgres:developer@localhost:5433/postgres?search_path=public&sslmode=disable"
  url = "postgres://postgres:developer@localhost:5433/postgres?search_path=public&sslmode=disable"
  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
    schema {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "dev" {
  dev = "postgres://postgres:developer@postgres-dev:5432/migrator?search_path=public&sslmode=disable"
  url = "postgres://postgres:developer@postgres-dev:5432/postgres?search_path=public&sslmode=disable"
  migration {
    dir = "file://migrations"
    exclude = [ "atlas_schema_revisions" ]
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
    schema {
      diff = "{{ sql . \"  \" }}"
    }
  }
}