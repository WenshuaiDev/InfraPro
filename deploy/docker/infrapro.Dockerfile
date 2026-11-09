FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/tools/infrapro
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY tools/infrapro /src/tools/infrapro
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/infrapro ./cmd/infrapro

FROM alpine:3.22
RUN addgroup -S infrapro && adduser -S -G infrapro infrapro
COPY --from=build /out/infrapro /usr/local/bin/infrapro
USER infrapro
ENTRYPOINT ["/usr/local/bin/infrapro"]
CMD ["--help"]
