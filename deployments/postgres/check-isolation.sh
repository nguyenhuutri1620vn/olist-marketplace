#!/bin/sh
# Checks AD-2: each service DB user can connect to its own database and to no
# other database. Runs psql inside the compose Postgres container by default.
set -u

DATABASES="identity catalog order payment shipping analytics"
PSQL="${PSQL:-docker compose -f deployments/docker-compose.yml exec -T postgres psql}"

failures=0
can_connect() { $PSQL -U "$1" -d "$2" -tAc 'SELECT 1' >/dev/null 2>&1; }

for user in $DATABASES; do
	for db in $DATABASES postgres template1; do
		if [ "$user" = "$db" ]; then
			can_connect "$user" "$db" || { echo "FAIL: $user cannot connect to own database"; failures=$((failures + 1)); }
		elif can_connect "$user" "$db"; then
			echo "FAIL: $user can connect to database $db"
			failures=$((failures + 1))
		fi
	done
done

if [ "$failures" -ne 0 ]; then
	echo "database isolation: $failures failure(s)"
	exit 1
fi
echo "database isolation: ok"
