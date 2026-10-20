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
networking=$("$client" --protocol=socket --connect-timeout=1 -uroot --batch --skip-column-names -e 'SELECT @@skip_networking')
if [ "$networking" != 0 ]; then
    printf '%s\n' 'MySQL/MariaDB initialization server is still active' >&2
    exit 1
fi
