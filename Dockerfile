FROM golang:1.17 as builder
WORKDIR     /src/k8s-cronjob
COPY        . .
RUN CGO_ENABLED=0 go build -o /app/k8s-cronjob .
FROM alpine:3.15
COPY --from=builder /app /app
ENTRYPOINT [ "/app/k8s-cronjob"]