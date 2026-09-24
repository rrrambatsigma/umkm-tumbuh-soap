ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/soap-partnerships-api ./cmd

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/soap-partnerships-api /app/soap-partnerships-api
COPY wsdl/ /app/wsdl/

USER appuser

EXPOSE 9090

CMD ["/app/soap-partnerships-api"]
