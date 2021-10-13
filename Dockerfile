FROM golang:1.16 AS build

WORKDIR /builder

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /traffic-deflector

FROM ubuntu:focal

COPY --from=build /traffic-deflector /usr/local/bin/

EXPOSE 8080

CMD ["/usr/local/bin/traffic-deflector"]
