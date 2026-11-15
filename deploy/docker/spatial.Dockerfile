# Shared API/worker image when spatial conversion is enabled. No samples or
# credentials are built into the image; the worker uses the same sealed contract.
FROM golang:1.27.0-alpine AS build
ENV GOWORK=off
WORKDIR /src/services/api
COPY packages/go/foundation /src/packages/go/foundation
COPY packages/go/platform /src/packages/go/platform
COPY services/api /src/services/api
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/spatial-worker ./cmd/spatial-worker

FROM python:3.12-slim-trixie
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 ca-certificates && rm -rf /var/lib/apt/lists/*
COPY tools/spatial-converter/requirements.lock /opt/spatial/requirements.lock
RUN pip install --no-cache-dir --require-hashes -r /opt/spatial/requirements.lock && useradd --system --uid 10001 --create-home infrapro
COPY tools/spatial-converter/convert.py /opt/spatial/convert.py
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/spatial-worker /usr/local/bin/spatial-worker
ENV INFRAPRO_SPATIAL_PYTHON=/usr/local/bin/python INFRAPRO_SPATIAL_CONVERTER_SCRIPT=/opt/spatial/convert.py INFRAPRO_SPATIAL_TEMP_DIRECTORY=/tmp
USER infrapro
RUN python -I /opt/spatial/convert.py --check
EXPOSE 8080
CMD ["/usr/local/bin/api"]
