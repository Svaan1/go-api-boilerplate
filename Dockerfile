# syntax=docker/dockerfile:1.7
FROM golang:1.25.13 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY config ./config
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/svaan1/go-api-boilerplate" \
      org.opencontainers.image.title="go-api-boilerplate"
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /src/config /app/config
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
