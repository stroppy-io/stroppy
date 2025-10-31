FROM postgres:17
COPY entrypoint.sh /docker-entrypoint-initdb.d/