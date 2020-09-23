FROM golang:1.15 AS builder
WORKDIR /go/src/github.com/wbh1/grafana-sqlite-to-postgres/
COPY . .
RUN make

FROM golang:1.15
WORKDIR /root/
COPY --from=builder /go/src/github.com/wbh1/grafana-sqlite-to-postgres/dist/grafana-migrate_linux* ./grafana-migrate
RUN apt-get update && apt-get install -y \
    sqlite3 \
 && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["./grafana-migrate"]
