set -eu
if command -v mariadb >/dev/null 2>&1; then
    client=mariadb
elif command -v mysql >/dev/null 2>&1; then
    client=mysql
else
    printf '%s\n' 'Neither mariadb nor mysql client is available' >&2
    exit 127
fi
export MYSQL_PWD="${MARIADB_ROOT_PASSWORD-${MYSQL_ROOT_PASSWORD?Set MARIADB_ROOT_PASSWORD or MYSQL_ROOT_PASSWORD}}"
user="${MARIADB_USER-${MYSQL_USER?Set MARIADB_USER or MYSQL_USER}}"
for db in "$@"; do
    "$client" --protocol=socket -uroot <<SQL
CREATE DATABASE IF NOT EXISTS \`$db\`;
GRANT ALL PRIVILEGES ON \`$db\`.* TO '$user'@'%';
SQL
done
"$client" --protocol=socket -uroot -e "FLUSH PRIVILEGES;"
