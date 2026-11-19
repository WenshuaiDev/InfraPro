FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/tools/infrapro
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/diagnostics /src/services/diagnostics
COPY tools/infrapro /src/tools/infrapro
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/infrapro ./cmd/infrapro

FROM alpine:3.22
RUN addgroup -g 101 -S infrapro && adduser -u 100 -S -G infrapro infrapro
COPY --from=build /out/infrapro /usr/local/bin/infrapro
USER 100:101
ENTRYPOINT ["/usr/local/bin/infrapro"]
CMD ["--help"]
