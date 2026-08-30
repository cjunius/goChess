# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/gochess ./cmd/gochess

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gochess /usr/local/bin/gochess
# UCI speaks over stdin/stdout; keep the container attached.
ENTRYPOINT ["/usr/local/bin/gochess"]
