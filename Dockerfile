FROM golang:1.24.2-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 go build -o /server-app -ldflags="-s -w" .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /server-app .

EXPOSE 8080

CMD ["./server-app"]