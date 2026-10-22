FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -o /payment-service ./cmd/api

FROM alpine:3.22
ENV APP_PORT=8080
EXPOSE 8080
COPY --from=build /payment-service /payment-service
ENTRYPOINT ["/payment-service"]
