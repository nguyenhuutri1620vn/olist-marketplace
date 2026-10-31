#!/bin/sh
# Creates one database and one owner user per data-owning service (AD-2).
# Runs once, when the Postgres data volume is first initialised.
# A user's password comes from <NAME>_DB_PASSWORD (e.g. CATALOG_DB_PASSWORD).
set -eu

DATABASES="identity catalog order payment shipping analytics"

password_of() {
	printenv "$(echo "$1" | tr '[:lower:]' '[:upper:]')_DB_PASSWORD" || true
}

# Check every password first so a missing one leaves nothing half-created.
for name in $DATABASES; do
	if [ -z "$(password_of "$name")" ]; then
		echo "01-databases.sh: $(echo "$name" | tr '[:lower:]' '[:upper:]')_DB_PASSWORD is not set" >&2
		exit 1
	fi
done

# Nobody but the owner may connect to a service database, or to the defaults.
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres <<SQL
REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;
REVOKE CONNECT ON DATABASE template1 FROM PUBLIC;
SQL

for name in $DATABASES; do
	password="$(password_of "$name")"
	psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
		-v name="$name" -v password="$password" <<'SQL'
CREATE ROLE :"name" LOGIN PASSWORD :'password';
CREATE DATABASE :"name" OWNER :"name";
REVOKE ALL ON DATABASE :"name" FROM PUBLIC;
SQL
done
