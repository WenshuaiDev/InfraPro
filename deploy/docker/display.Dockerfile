# Build shell on the host first: pnpm --filter @infrapro/display-runtime build
FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/display-runtime ./cmd/display-runtime && \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/display-assets ./cmd/display-assets && \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/display-package-worker ./cmd/display-package-worker
FROM alpine:3.22
RUN addgroup -g 101 -S infrapro && adduser -u 100 -S -G infrapro infrapro
COPY --from=build /out/ /usr/local/bin/
COPY apps/display-runtime/dist /opt/display-shell
ENV INFRAPRO_DISPLAY_SHELL_DIRECTORY=/opt/display-shell INFRAPRO_DISPLAY_RUNTIME_ADDR=0.0.0.0:5180 INFRAPRO_DISPLAY_ASSET_ADDR=0.0.0.0:5181
USER infrapro
EXPOSE 5180 5181
CMD ["/usr/local/bin/display-runtime"]
