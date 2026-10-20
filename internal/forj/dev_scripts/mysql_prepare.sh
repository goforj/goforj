set -eu
for db in "$@"; do
    mariadb --protocol=socket -uroot -p"$MARIADB_ROOT_PASSWORD" <<SQL
CREATE DATABASE IF NOT EXISTS \`$db\`;
GRANT ALL PRIVILEGES ON \`$db\`.* TO '$MARIADB_USER'@'%';
SQL
done
mariadb --protocol=socket -uroot -p"$MARIADB_ROOT_PASSWORD" -e "FLUSH PRIVILEGES;"
