FROM golang:1.15 AS builder
WORKDIR /go/src/github.com/wbh1/grafana-sqlite-to-postgres/
COPY . .
RUN make

FROM golang:1.15
WORKDIR /root/
COPY --from=builder /go/src/github.com/wbh1/grafana-sqlite-to-postgres/dist/grafana-migrate_linux* ./grafana-migrate
RUN mkdir /tmp/sqlite3 \
    && cd /tmp/sqlite3/ \
    && curl -sSfL https://www.sqlite.org/2022/sqlite-autoconf-3390200.tar.gz | tar zxv --strip-components 1 \
    && ./configure \
    && make -j \
    && make install \
    && cd ~ \
    && rm -rf /tmp/sqlite3
ENTRYPOINT ["./grafana-migrate"]
