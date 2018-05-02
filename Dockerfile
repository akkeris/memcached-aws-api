FROM golang:1.5-alpine
RUN apk update
RUN apk add openssl ca-certificates git
RUN mkdir -p /go/src/oct-redis-api
ADD server.go  /go/src/oct-redis-api/server.go
ADD build.sh /build.sh
RUN chmod +x /build.sh
RUN /build.sh
#RUN mkdir /root/.aws
#ADD credentials /root/.aws/credentials
CMD ["/go/src/oct-redis-api/server"]
EXPOSE 3000

