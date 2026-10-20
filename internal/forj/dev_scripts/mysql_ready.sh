set -eu
export MYSQL_PWD="$MARIADB_ROOT_PASSWORD"
networking=$(mariadb --protocol=socket --connect-timeout=1 -uroot --batch --skip-column-names -e 'SELECT @@skip_networking')
if [ "$networking" != 0 ]; then
    printf '%s\n' 'MariaDB initialization server is still active' >&2
    exit 1
fi
