FROM golang:1.17 AS builder
WORKDIR /go/src/github.com/percona/grafana-db-migrator/
COPY . .
RUN make

FROM golang:1.17
WORKDIR /root/
COPY --from=builder /go/src/github.com/percona/grafana-db-migrator/dist/grafana-db-migrator ./grafana-migrate
RUN apt-get update && apt-get install -y \
    sqlite3 \
 && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["./grafana-migrate"]
