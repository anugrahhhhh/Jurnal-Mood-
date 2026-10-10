# Build: binary Go statis. Template, CSS, JS, dan migrasi sudah di-embed.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/moodjournal ./cmd/server

# Runtime: image minimal tanpa shell, berjalan sebagai pengguna non-root.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/moodjournal /app/moodjournal
# Contoh kontak krisis dipakai jika config/crisis.yaml belum dipasang (halaman
# bantuan akan menampilkan peringatan "belum diverifikasi").
COPY config/crisis.example.yaml /app/config/crisis.example.yaml
ENV ADDR=0.0.0.0:8080 \
    APP_ENV=production \
    CRISIS_FILE=/app/config/crisis.yaml
USER nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/app/moodjournal", "healthcheck"]
ENTRYPOINT ["/app/moodjournal"]
