FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath \
    -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$BUILD_DATE" \
    -o /out/lumbrera ./cmd/lumbrera

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
RUN apk add --no-cache git ca-certificates \
    && addgroup -g 10001 lumbrera \
    && adduser -D -u 10001 -G lumbrera lumbrera \
    && mkdir /data && chown 10001:10001 /data
COPY --from=build /out/lumbrera /usr/local/bin/lumbrera
ARG VERSION=dev
ARG COMMIT=none
LABEL org.opencontainers.image.title="Lumbrera" \
      org.opencontainers.image.source="https://github.com/javiermolinar/lumbrera" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$COMMIT
USER 10001:10001
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["lumbrera"]
CMD ["--help"]
