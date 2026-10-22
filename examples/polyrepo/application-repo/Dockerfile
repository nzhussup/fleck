FROM gcr.io/distroless/static-debian12:nonroot
ENV REDIS_ADRESS=redis://cache.example:6379
ENV HTTP_PORT=8080
EXPOSE 8080
ENTRYPOINT ["/customer-api"]
