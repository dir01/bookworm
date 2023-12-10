FROM golang:1.21

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y calibre && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .

ENV CGO_ENABLED=1
RUN go build --tags "fts5" -o ./bookworm .

RUN mkdir /app/books

CMD /app/bookworm
