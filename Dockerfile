FROM docker.io/golang:1.27-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /build/teleport-exporter ./cmd/teleport-exporter

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /build/teleport-exporter /usr/local/bin/teleport-exporter

USER 65535:65535

EXPOSE 9180

ENTRYPOINT ["/usr/local/bin/teleport-exporter"]
