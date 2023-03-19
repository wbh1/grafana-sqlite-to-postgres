FROM golang:1.18 AS builder
WORKDIR /go/src/github.com/wbh1/grafana-sqlite-to-postgres/
COPY . .
RUN rm -rf /go/src/github.com/wbh1/grafana-sqlite-to-postgres/dist/grafana-migrate_linux*
RUN make
RUN ls /go/src/github.com/wbh1/grafana-sqlite-to-postgres/dist/grafana-migrate_linux*refs/tags/*

FROM golang:1.18
WORKDIR /root/
COPY --from=builder /go/src/github.com/wbh1/grafana-sqlite-to-postgres/dist/grafana-migrate_linux*+refs/tags/v* ./grafana-migrate
RUN apt-get update && apt-get install -y \
    sqlite3 \
 && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["./grafana-migrate"]
