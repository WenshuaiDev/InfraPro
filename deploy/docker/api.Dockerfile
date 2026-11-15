FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/monitoring-worker ./cmd/monitoring-worker
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/notification-worker ./cmd/notification-worker
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/task-index-worker ./cmd/task-index-worker

RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/files-worker ./cmd/files-worker
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/files-checker ./cmd/files-checker

FROM alpine:3.22
RUN addgroup -S infrapro && adduser -S -G infrapro infrapro
RUN apk add --no-cache ca-certificates
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/monitoring-worker /usr/local/bin/monitoring-worker
COPY --from=build /out/notification-worker /usr/local/bin/notification-worker
COPY --from=build /out/task-index-worker /usr/local/bin/task-index-worker
COPY --from=build /out/files-worker /usr/local/bin/files-worker
COPY --from=build /out/files-checker /usr/local/bin/files-checker
USER infrapro
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
