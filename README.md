## Synopsis

Docker image which runs an http server with REST interface for provisioning of memcached clusters on AWS ElastiCache

## Details

Listens on Port 3000
Supports the following

1. GET /v1/memcached/plans
2. POST /v1/memcached/instance/ with JSON data of plan and billingcode
3. DELETE /v1/memcached/intance/:name
4. GET /v1/memcached/url/:name

## Runtime Environment Variables

1. ACCOUNTNUMBER - e.g., aws account number
2. BROKERDB - postgres:// database
3. REGION - e.g., us-west-2

