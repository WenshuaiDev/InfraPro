FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN mkdir /out && CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/ ./cmd/...

FROM alpine:3.22
RUN addgroup -g 101 -S infrapro && adduser -u 100 -S -G infrapro infrapro && apk add --no-cache ca-certificates
COPY --from=build /out/ /usr/local/bin/
USER 100:101
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
