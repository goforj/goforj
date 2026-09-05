FROM mariadb:10.11.6@sha256:e846701e66dcc95a3f90d161cc3dfc2e4ff96f0699ee4fb93b0bfdab94158570

COPY my.cnf /etc/mysql/conf.d/my.cnf

ARG INNODB_BUFFER_POOL_SIZE="256MB"

RUN echo "[mysqld]" >> /etc/mysql/conf.d/my.cnf && \
	echo "innodb_buffer_pool_size  = ${INNODB_BUFFER_POOL_SIZE}" >> /etc/mysql/conf.d/my.cnf

EXPOSE 3306
