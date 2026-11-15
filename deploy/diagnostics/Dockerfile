FROM node:26.10.0-alpine AS ui
WORKDIR /src
RUN npm install -g pnpm@12.18.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/ts ./packages/ts
COPY apps/diagnostics ./apps/diagnostics
COPY scripts/diagnostics-build.mjs ./scripts/diagnostics-build.mjs
RUN pnpm install --frozen-lockfile --filter @infrapro/diagnostics...
RUN pnpm --filter @infrapro/diagnostics build
RUN node scripts/diagnostics-build.mjs --skip-ui --embed-only
RUN node -e 'const fs=require("fs");fs.writeFileSync("/build-id",JSON.parse(fs.readFileSync("services/diagnostics/internal/web/assets/build.json")).build_id)'

FROM golang:1.27.0-alpine AS build
ENV GOWORK=off CGO_ENABLED=0
WORKDIR /src/services/diagnostics
COPY packages/go/foundation /src/packages/go/foundation
COPY services/diagnostics /src/services/diagnostics
COPY --from=ui /src/services/diagnostics/internal/web/assets ./internal/web/assets
COPY --from=ui /build-id /build-id
RUN go build -mod=readonly -trimpath -tags diagnostics_release -ldflags "-X github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web.BuildID=$(cat /build-id)" -o /out/infrapro-diagnostics ./cmd/infrapro-diagnostics

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -g 10001 diagnostics && adduser -D -u 10001 -G diagnostics diagnostics
COPY --from=build /out/infrapro-diagnostics /usr/local/bin/infrapro-diagnostics
USER 10001:10001
RUN /usr/local/bin/infrapro-diagnostics version --verify-assets
EXPOSE 8096
ENTRYPOINT ["/usr/local/bin/infrapro-diagnostics"]
