# syntax=docker/dockerfile:1
FROM oven/bun:1.4.4-alpine AS bun
FROM node:26.13.1-alpine3.24 AS development
ARG LOCAL_UID=1000
ARG LOCAL_GID=1000
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
ENV BUN_INSTALL_CACHE_DIR=/tmp/bun-cache OPEN_BROWSER=false
WORKDIR /app/frontend
COPY app/frontend/package.json app/frontend/bun.lock ./
RUN bun install --frozen-lockfile && chown -R ${LOCAL_UID}:${LOCAL_GID} /app/frontend /tmp/bun-cache
COPY --chown=${LOCAL_UID}:${LOCAL_GID} app/frontend/ ./
USER ${LOCAL_UID}:${LOCAL_GID}
EXPOSE 3000
CMD ["sh", "-c", "bun install --frozen-lockfile && exec node scripts/dev.ts --host 0.0.0.0 --port 3000 --backend-host backend --backend-port 8080 --no-open"]
