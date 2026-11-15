FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/files-worker ./cmd/files-worker && CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/files-checker ./cmd/files-checker
FROM alpine:3.22
RUN addgroup -g 10001 -S infrapro && adduser -u 10001 -S -G infrapro infrapro && apk add --no-cache ca-certificates && mkdir /staging && chown 10001:10001 /staging
COPY --from=build /out/ /usr/local/bin/
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/files-worker"]
