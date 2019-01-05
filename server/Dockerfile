FROM golang:1.11.4-stretch

ADD . /vchamber

WORKDIR /vchamber

RUN go build -mod=vendor -o server

ENTRYPOINT ["/vchamber/server"]

EXPOSE 8080 8081