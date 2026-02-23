---
name: docker
description: "Build, run, and manage containers with Docker. Images, containers, volumes, networks, Compose, multi-stage builds, and troubleshooting."
metadata: {"nanobot":{"emoji":"🐳","requires":{"bins":["docker"]},"install":[{"id":"brew","kind":"brew","formula":"docker","bins":["docker"],"label":"Install Docker (brew)"}]}}
---

# Docker Skill

Use the `docker` CLI to build, run, and manage containers and images.

## Containers

```bash
# List running containers
docker ps

# List all containers (including stopped)
docker ps -a

# Run a container
docker run -d --name my-app -p 8080:80 nginx:latest

# Run interactive
docker run -it --rm ubuntu:22.04 bash

# Run with environment variables and volumes
docker run -d --name my-app \
  -p 8080:3000 \
  -e DATABASE_URL="postgres://..." \
  -e NODE_ENV=production \
  -v $(pwd)/data:/app/data \
  --restart unless-stopped \
  myapp:latest

# Stop / start / restart
docker stop my-app
docker start my-app
docker restart my-app

# Remove a container
docker rm my-app
docker rm -f my-app  # force-stop and remove

# View logs
docker logs my-app
docker logs -f my-app           # follow
docker logs --tail 100 my-app   # last 100 lines
docker logs --since 10m my-app  # last 10 minutes

# Exec into a running container
docker exec -it my-app bash
docker exec -it my-app sh  # for Alpine-based images
docker exec my-app cat /etc/hostname

# Inspect a container
docker inspect my-app
docker inspect --format='{{.State.Status}}' my-app
docker inspect --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' my-app

# Resource usage
docker stats
docker stats my-app --no-stream

# Copy files in/out
docker cp my-app:/var/log/app.log ./app.log
docker cp ./config.json my-app:/app/config.json
```

## Images

```bash
# List images
docker images

# Pull an image
docker pull nginx:latest
docker pull ubuntu:22.04

# Build an image
docker build -t myapp:latest .
docker build -t myapp:v2.1.0 -f Dockerfile.prod .

# Build with build args
docker build -t myapp:latest --build-arg VERSION=2.1.0 .

# Tag an image
docker tag myapp:latest registry.example.com/myapp:v2.1.0

# Push to registry
docker push registry.example.com/myapp:v2.1.0

# Remove an image
docker rmi myapp:old-tag

# Prune unused images
docker image prune -a  # remove all unused images

# Image history (layers)
docker history myapp:latest

# Save/load images (for offline transfer)
docker save myapp:latest | gzip > myapp.tar.gz
docker load < myapp.tar.gz
```

## Multi-stage Build (Dockerfile)

```dockerfile
# Build stage
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Production stage
FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
EXPOSE 3000
USER node
CMD ["node", "dist/index.js"]
```

## Volumes

```bash
# List volumes
docker volume ls

# Create a volume
docker volume create app-data

# Inspect a volume
docker volume inspect app-data

# Remove unused volumes
docker volume prune
```

## Networks

```bash
# List networks
docker network ls

# Create a network
docker network create app-net

# Connect a container to a network
docker network connect app-net my-app

# Inspect a network
docker network inspect app-net
```

## Docker Compose

```bash
# Start all services
docker compose up -d

# Start specific service
docker compose up -d web

# Stop all services
docker compose down

# Stop and remove volumes
docker compose down -v

# View logs
docker compose logs -f
docker compose logs web --tail 50

# Restart a service
docker compose restart web

# Scale a service
docker compose up -d --scale worker=3

# Build and start
docker compose up -d --build

# Exec into a service
docker compose exec web bash

# List running services
docker compose ps

# Pull latest images
docker compose pull
```

### docker-compose.yaml example:
```yaml
services:
  web:
    build: .
    ports: ["8080:3000"]
    environment:
      - DATABASE_URL=postgres://db:5432/app
    depends_on: [db]
    restart: unless-stopped

  db:
    image: postgres:16-alpine
    volumes: [db-data:/var/lib/postgresql/data]
    environment:
      POSTGRES_DB: app
      POSTGRES_PASSWORD: secret

  redis:
    image: redis:7-alpine

volumes:
  db-data:
```

## Registry Operations

```bash
# Login to a registry
docker login
docker login registry.example.com

# Login to ECR
aws ecr get-login-password | docker login --username AWS --password-stdin ACCOUNT.dkr.ecr.REGION.amazonaws.com

# Login to GCR
gcloud auth configure-docker

# Login to ACR
az acr login --name myregistry
```

## Cleanup

```bash
# Remove all stopped containers
docker container prune

# Remove all unused images, networks, volumes
docker system prune -a --volumes

# Show disk usage
docker system df
```

## Debugging

```bash
# Check why a container exited
docker inspect --format='{{.State.ExitCode}}' my-app
docker logs my-app 2>&1 | tail -20

# Check container processes
docker top my-app

# Check resource limits
docker inspect --format='{{.HostConfig.Memory}}' my-app

# Run health check manually
docker inspect --format='{{.State.Health.Status}}' my-app
```

## Tips

- Use `--rm` for throwaway containers: `docker run --rm -it alpine sh`.
- Use `.dockerignore` to exclude files from build context.
- Use `HEALTHCHECK` in Dockerfiles for automatic health monitoring.
- Use `docker compose watch` for development with live reload.
- Pin image tags in production — never use `:latest` in deployments.
- Use `--no-cache` when debugging build issues: `docker build --no-cache .`.

## Bundled Scripts

- Cleanup dangling resources: `{baseDir}/scripts/cleanup-dangling.sh` (add `--dry-run` to preview)
- Wait for container health: `{baseDir}/scripts/wait-healthy.sh -c container_name`
