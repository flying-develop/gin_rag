# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.25 AS build

WORKDIR /src

# Слой зависимостей кэшируется, пока не меняются go.mod / go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Статический бинарь без CGO — чтобы работал в distroless/static.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/api /app/api

EXPOSE 8080
USER nonroot:nonroot

# В distroless нет curl/wget — проба идёт самим бинарём (подкоманда healthcheck).
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD ["/app/api", "healthcheck"]

ENTRYPOINT ["/app/api"]
