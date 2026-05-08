FROM golang:1.21
WORKDIR /app

RUN apt-get update && apt-get install -y gcc sqlite3 libsqlite3-dev

COPY relay/go.mod relay/go.sum ./
RUN go mod download

COPY relay/ .
RUN go build -o relay .

CMD ["./relay"]
