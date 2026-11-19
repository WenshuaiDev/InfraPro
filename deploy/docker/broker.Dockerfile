# Build using the verified offline release directory as context.
FROM alpine:3.22
COPY bin/nats-server bin/event-relay /usr/local/bin/
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/nats-server","-c","/etc/infrapro/broker/nats.conf"]
HEALTHCHECK --interval=10s --timeout=4s CMD ["/usr/local/bin/event-relay","broker-health"]
