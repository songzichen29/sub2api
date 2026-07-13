# Apple container Deployment

Sub2API can run as a native three-service stack with Apple's `container` CLI. This workflow runs the configured Sub2API, MySQL 8, and Redis OCI images without Docker Desktop or a Docker-compatible daemon.

## Support Level

Apple `container` support is intended for local development and operator-managed deployments on a Mac. Docker Compose remains the recommended production deployment path.

Apple `container` 1.1 does not provide restart policies, automatic startup, workload health scheduling, a Docker API socket, or full Compose orchestration. `apple-container.sh` supplies ordered startup and readiness checks when you invoke it, but it is not a continuously running supervisor.

## Requirements

- A Mac with Apple silicon
- macOS 26 or newer
- Apple `container` 1.1.0 or newer
- `openssl` for generating initial secrets
- Local Network access for `container-runtime-linux` when macOS prompts during the first published-container startup

Install Apple `container` from its [official releases](https://github.com/apple/container/releases), then verify it:

```bash
container --version
```

## Quick Start

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api/deploy

# Creates .env with random MySQL, JWT, and TOTP secrets.
./apple-container.sh init

# Review optional settings before startup.
nano .env

# Creates volumes/network/containers, waits for dependencies, and starts Sub2API.
./apple-container.sh up

# Verifies MySQL, Redis, and the application endpoint.
./apple-container.sh status
```

Open `http://localhost:8080`. If `ADMIN_PASSWORD` is empty, retrieve the generated password with:

```bash
./apple-container.sh logs app
```

The env file uses literal `KEY=value` syntax. Do not use Compose expressions such as `${VALUE:-default}`, and do not quote values unless the quote characters are part of the intended value. `BIND_HOST` must be an IPv4 address, and `SERVER_PORT` must be between 1025 and 65535.

## Commands

```bash
# Start dependencies and recreate the lightweight app container with current IPs.
./apple-container.sh up

# Also recreate MySQL and Redis containers, preserving their volumes.
./apple-container.sh up --recreate

# Stop containers while preserving all resources and data.
./apple-container.sh down

# Restart MySQL, Redis, and Sub2API in dependency order.
./apple-container.sh restart

# Show resource state and run live health probes.
./apple-container.sh status

# Follow one service's logs.
./apple-container.sh logs app -f
./apple-container.sh logs mysql -f
./apple-container.sh logs redis -f

# Pull all configured images for linux/arm64, then recreate containers.
./apple-container.sh pull
./apple-container.sh up --recreate

# Delete containers and the network, preserving named volumes.
./apple-container.sh destroy --yes

# Permanently delete the stack and all application/database/cache data.
./apple-container.sh destroy --volumes --yes
```

`destroy --volumes` does not remove `.env`, backup files, or pulled images. Delete credentials and backups separately when decommissioning a deployment. Use `container image delete <image>` only after confirming no other Apple containers use that image.

After a host reboot or `container system stop`, run `./apple-container.sh up` again. Apple `container` does not automatically restart persisted containers.

## Configuration

The script uses `deploy/.env`, the same source file used by Docker Compose. Export `SUB2API_ENV_FILE` to use another file for every command in the current shell:

```bash
export SUB2API_ENV_FILE=/absolute/path/to/sub2api.env
./apple-container.sh init
./apple-container.sh up
```

Apple-specific image overrides are available:

```dotenv
APPLE_CONTAINER_SUB2API_IMAGE=
APPLE_CONTAINER_MYSQL_IMAGE=mysql:8.0
APPLE_CONTAINER_REDIS_IMAGE=redis:8-alpine
```

When `APPLE_CONTAINER_SUB2API_IMAGE` is empty, the script uses `SUB2API_IMAGE_NAME`. The normal `up` command recreates the application container, so application environment changes are applied immediately. Use `up --recreate` when changing MySQL or Redis container images or Redis runtime configuration. Persistent data remains in named volumes.

`DATABASE_USER`, `DATABASE_PASSWORD`, and `DATABASE_DBNAME` are mapped to `MYSQL_USER`, `MYSQL_PASSWORD`, and `MYSQL_DATABASE` when MySQL initializes an empty data volume. `DATABASE_PASSWORD` is also used as `MYSQL_ROOT_PASSWORD`, so no second database password is required. Changing these values in `.env` and recreating the container does not change an existing database. Rotate passwords with `ALTER USER`, and plan explicit migrations for user or database changes. To intentionally initialize a new empty database, first back up the old one and use `destroy --volumes`.

Apple-specific handling of shared settings:

| Setting | Apple workflow behavior |
|---|---|
| Application and gateway variables | Passed to Sub2API from `.env` |
| `BIND_HOST`, `SERVER_PORT` | Used for the macOS published port |
| `DATABASE_USER`, `DATABASE_PASSWORD`, `DATABASE_DBNAME` | Passed to Sub2API and mapped to MySQL initialization variables |
| `REDIS_PASSWORD` | Applied to Redis and Sub2API |
| `DATABASE_HOST`, `DATABASE_PORT`, `REDIS_HOST`, `REDIS_PORT` | Dependency addresses are injected; internal ports are fixed to 3306 and 6379 |
| `REDIS_MAXCLIENTS` | Not currently applied to the cache server |

## Managed Resources

The script creates only resources carrying the `org.sub2api.stack=apple-container` label:

| Type | Names |
|---|---|
| Containers | `sub2api-apple`, `sub2api-apple-mysql`, `sub2api-apple-redis` |
| Network | `sub2api-apple` |
| Volumes | `sub2api-apple-data`, `sub2api-apple-mysql-data`, `sub2api-apple-redis-data` |

The MySQL volume is mounted at `/var/lib/mysql`. Sub2API and Redis store data in child directories below their Apple volume mount points. The explicit initialization and ownership handling is required because Apple named volumes do not have Docker's copy-up and mount-point ownership behavior.

## Networking

Apple `container` 1.1 does not provide Compose-style network-scoped service aliases. After MySQL and Redis start, the script reads their current private-network IPv4 addresses from `container inspect`, injects those addresses into a newly created application container, and then starts Sub2API. The script does not modify `~/.config/container/config.toml` or the macOS host resolver.

All three services attach only to the private `sub2api-apple` network. Only the application publishes a host port; database and Redis ports remain unpublished.

The application container is intentionally recreated by every `up` and `restart` operation because dependency VM addresses can change after they stop. Application data remains in `sub2api-apple-data`.

The script checks the published `/health` endpoint from macOS before reporting success. Approve the Local Network prompt on first startup. If the internal probe succeeds but the host-port probe fails with a connection reset, enable Local Network access for `container-runtime-linux`, run `container system stop` followed by `container system start`, and then run `up` again. Runtime upgrades may prompt for permission again.

## Backup and Upgrade

Pin image release tags or digests in `.env` before using this workflow for persistent data. Before an application or database image upgrade, create backups while the stack is healthy:

```bash
umask 077
mkdir -p backups

# Logical MySQL backup.
container exec sub2api-apple-mysql sh -c \
  'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysqldump --host=127.0.0.1 --user=root --single-transaction --routines --triggers "$MYSQL_DATABASE"' \
  > backups/sub2api.sql

# Application configuration and local files.
container exec sub2api-apple sh -c 'tar -C "$DATA_DIR" -czf - .' \
  > backups/sub2api-data.tar.gz

./apple-container.sh pull
./apple-container.sh up --recreate
./apple-container.sh status
```

Database migrations are forward-only. Keep the previous image reference and both backups until the upgraded stack has been validated; image rollback alone cannot reverse a migrated database. Test restore procedures before relying on this workflow for important data.

To restore these backups into an existing stack, first ensure the image versions are compatible with the backup, then stop writers and replace both data sets:

```bash
# Ensure empty/current resources exist, then stop the stack.
./apple-container.sh up
./apple-container.sh down

# Remove only the app container so a helper can mount its named volume.
container delete sub2api-apple
SUB2API_IMAGE=ghcr.io/your-github-owner/sub2api:latest # Match the resolved Apple Sub2API image.
container run --rm --name sub2api-apple-data-restore \
  --entrypoint /bin/sh \
  --volume sub2api-apple-data:/restore \
  --volume "$PWD/backups:/backup:ro" \
  "$SUB2API_IMAGE" \
  -c 'rm -rf /restore/data && mkdir -p /restore/data && tar -xzf /backup/sub2api-data.tar.gz -C /restore/data'

# Restore the logical database while the application is absent.
container start sub2api-apple-mysql
until container exec sub2api-apple-mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysqladmin ping --protocol=tcp --host=127.0.0.1 --user="$MYSQL_USER" --silent'; do sleep 1; done
container copy backups/sub2api.sql sub2api-apple-mysql:/tmp/sub2api.sql
container exec sub2api-apple-mysql sh -c '
  MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --protocol=tcp --host=127.0.0.1 --user=root \
    -e "DROP DATABASE IF EXISTS \`$MYSQL_DATABASE\`; CREATE DATABASE \`$MYSQL_DATABASE\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
  MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=tcp --host=127.0.0.1 --user="$MYSQL_USER" "$MYSQL_DATABASE" < /tmp/sub2api.sql
  rm /tmp/sub2api.sql
'

./apple-container.sh up
./apple-container.sh status
```

For disaster recovery after deleting the named volumes, run `up` once to create a fresh stack before following the restore sequence. Perform restore drills with non-production data first.

To upgrade the Apple runtime itself:

```bash
./apple-container.sh down
container system stop
# Install/update Apple container 1.1.0 or newer.
container system start
./apple-container.sh up
```

## Operational Limitations

- There is no `restart: unless-stopped` equivalent. Run `up` after reboot, or add your own launchd supervisor.
- Health probes run during `up`, `restart`, and `status`; Apple `container` does not continuously schedule them.
- Docker Compose, Testcontainers, Buildx, and tools requiring `/var/run/docker.sock` cannot use this runtime directly.
- Named volume backup and restore must be tested before using this workflow for important data.
- The script targets native `linux/arm64` images. The normal Sub2API release publishes an arm64 variant.
- Runtime environment values, including credentials, are retained in Apple container configuration and are visible to users who can inspect the local runtime.
