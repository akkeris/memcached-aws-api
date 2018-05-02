## Synopsis

Docker image which runs an http server with REST interface for provisioning of memcached clusters on AWS ElastiCache

## Details

Listens on Port 3000
Supports the following

1. GET /v1/memcached/plans
2. POST /v1/memcached/instance/ with JSON data of plan and billingcode
3. DELETE /v1/memcached/intance/:name
4. GET /v1/memcached/url/:name


## Dependencies

1. "github.com/go-martini/martini"
2. "github.com/martini-contrib/render"
3. "github.com/martini-contrib/binding"
4. "github.com/aws/aws-sdk-go/aws"
5. "github.com/aws/aws-sdk-go/aws/session"
6. "github.com/aws/aws-sdk-go/service/elasticache"
7. "fmt"
8. "strconv"
9. "database/sql"
10. "github.com/lib/pq"
11. "os"



## Requirements
go

aws creds

## Runtime Environment Variables
1. ACCOUNTNUMBER
2. BROKERDB
3. REGION

