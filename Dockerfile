# syntax=docker/dockerfile:1

# ---------- build ----------
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=1.0.0 COMMIT=unknown BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X cholobangladesh-server/pkg/buildinfo.Version=${VERSION} -X cholobangladesh-server/pkg/buildinfo.Commit=${COMMIT} -X cholobangladesh-server/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/manage ./cmd/manage

# ---------- runtime ----------
# distroless: no shell, no package manager, runs as non-root, includes CA certs.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
COPY --from=build /out/manage /usr/local/bin/manage

ENV APP_ENV=production PORT=8080
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]
