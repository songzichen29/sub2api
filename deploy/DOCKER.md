# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

## Quick Start

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e AUTO_SETUP=true \
  -e DATABASE_HOST=mysql.example.internal \
  -e DATABASE_PORT=3306 \
  -e DATABASE_USER=sub2api \
  -e DATABASE_PASSWORD=change_this_password \
  -e DATABASE_DBNAME=sub2api \
  -e REDIS_HOST=redis.example.internal \
  -e REDIS_PORT=6379 \
  weishaw/sub2api:latest
```

## Docker Compose

```yaml
services:
  sub2api:
    image: weishaw/sub2api:latest
    ports:
      - "8080:8080"
    environment:
      - AUTO_SETUP=true
      - DATABASE_HOST=mysql
      - DATABASE_PORT=3306
      - DATABASE_USER=sub2api
      - DATABASE_PASSWORD=sub2api
      - DATABASE_DBNAME=sub2api
      - DATABASE_SSLMODE=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy

  mysql:
    image: mysql:8.0
    command: --character-set-server=utf8mb4 --collation-server=utf8mb4_0900_ai_ci
    environment:
      - MYSQL_DATABASE=sub2api
      - MYSQL_USER=sub2api
      - MYSQL_PASSWORD=sub2api
      - MYSQL_ROOT_PASSWORD=sub2api
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -usub2api -psub2api --silent"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:8-alpine
    volumes:
      - redis_data:/data

volumes:
  mysql_data:
  redis_data:
```

## Startup and Database Recovery

Sub2API runs database migrations while starting. PostgreSQL may still be
recovering briefly after a host or Docker daemon restart. The application
retries transient PostgreSQL startup and connection errors with bounded
exponential backoff, then continues startup when the database is ready.
Permanent errors such as invalid credentials, migration checksum mismatches,
SQL errors, and incompatible data fail immediately.

The Compose deployment also checks PostgreSQL readiness with both `pg_isready`
and a simple SQL query. `depends_on: condition: service_healthy` helps order a
fresh Compose start, but application-level retries are still required when
Docker restores existing containers after a host restart.

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_HOST` | MySQL host | Yes | - |
| `DATABASE_PORT` | MySQL port | No | `3306` |
| `DATABASE_USER` | MySQL user | Yes | - |
| `DATABASE_PASSWORD` | MySQL password | Yes | - |
| `DATABASE_DBNAME` | MySQL database name | Yes | - |
| `DATABASE_SSLMODE` | MySQL TLS mode | No | `disable` |
| `REDIS_HOST` | Redis host | Yes | - |
| `REDIS_PORT` | Redis port | No | `6379` |
| `SERVER_PORT` | Server port inside the container | No | `8080` |
| `SERVER_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Links

- [GitHub Repository](https://github.com/weishaw/sub2api)
- [Documentation](https://github.com/weishaw/sub2api#readme)
