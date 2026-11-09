FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/api ./cmd/api

FROM alpine:3.22
RUN addgroup -S infrapro && adduser -S -G infrapro infrapro
COPY --from=build /out/api /usr/local/bin/api
USER infrapro
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
