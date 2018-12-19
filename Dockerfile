FROM golang:1.8-alpine
RUN apk update
RUN apk add openssl ca-certificates git
RUN mkdir -p /go/src/oct-memcached-api
ADD server.go  /go/src/oct-memcached-api/server.go
ADD build.sh /build.sh
RUN chmod +x /build.sh
RUN /build.sh
#RUN mkdir /root/.aws
#ADD credentials /root/.aws/credentials
CMD ["/go/src/oct-memcached-api/server"]
EXPOSE 3000

