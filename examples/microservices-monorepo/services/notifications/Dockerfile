FROM gcr.io/distroless/static-debian12:nonroot
ENV QUEUE_URL=amqp://queue.example/notifications
ENV WORKER_CONCURRENCY=4
ENTRYPOINT ["/notifications"]
