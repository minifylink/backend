FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY .env ./

RUN apk add --no-cache git build-base curl

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go list -m all | grep jackc/pgx

RUN CGO_ENABLED=0 GOOS=linux go build -o backend ./cmd/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/backend .

RUN mkdir -p /app/data

EXPOSE 8082

CMD ["./backend"]
