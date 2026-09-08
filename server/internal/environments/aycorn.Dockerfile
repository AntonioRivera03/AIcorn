FROM node:24.13.0-bookworm-slim AS node
FROM golang:1.25.7-bookworm AS test
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm
WORKDIR /src
COPY app/package.json app/package-lock.json ./app/
RUN cd app && npm ci
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download
COPY . .
RUN cd app && npm run build:md-convert && npm run build
RUN chown -R 1000:1000 /src /go
ENV GOCACHE=/tmp/go-build GOMODCACHE=/go/pkg/mod

FROM test AS build
RUN cd server && CGO_ENABLED=0 go build -trimpath -o /out/aycorn ./cmd/web

FROM node:24.13.0-bookworm-slim AS preview
RUN mkdir -p /data && chown 1000:1000 /data
COPY --from=build /out/aycorn /usr/local/bin/aycorn
USER 1000:1000
ENV AYCORN_HOST=0.0.0.0 AYCORN_PORT=8000 AYCORN_DB=/data/app.db AYCORN_PREVIEW=1
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/aycorn"]
