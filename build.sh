#!/bin/sh

cd /go/src
go get  "github.com/lib/pq"
go get  "github.com/aws/aws-sdk-go/aws"
go get  "github.com/aws/aws-sdk-go/aws/session"
go get  "github.com/aws/aws-sdk-go/service/elasticache"
go get  "github.com/go-martini/martini"
go get  "github.com/martini-contrib/binding"
go get  "github.com/martini-contrib/render"
cd /go/src/oct-redis-api
go build server.go

