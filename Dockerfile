# Stage 1: Build UI
FROM node:22-alpine AS ui-builder
WORKDIR /app
COPY UI/package*.json ./
RUN npm ci
COPY UI/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.26.5-alpine AS be-builder
WORKDIR /app
COPY BE/go.mod BE/go.sum ./
RUN go mod download
COPY BE/ .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

# Stage 3: Final image
FROM alpine:3.23
RUN apk update && apk upgrade --no-cache && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=be-builder /app/server .
COPY --from=be-builder /app/migrations ./migrations
COPY --from=ui-builder /app/build /srv
RUN mkdir -p /app/uploads

EXPOSE 8080
CMD ["./server"]
