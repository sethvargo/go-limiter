module github.com/sethvargo/go-limiter/benchmarks

go 1.24

toolchain go1.24.6

replace github.com/sethvargo/go-limiter => ../

require (
	github.com/clipperhouse/rate v0.2.0
	github.com/didip/tollbooth/v6 v6.1.1
	github.com/gomodule/redigo v1.8.5
	github.com/sethvargo/go-limiter v0.6.0
	github.com/sethvargo/go-redisstore v0.3.0
	github.com/throttled/throttled v2.2.5+incompatible
	github.com/ulule/limiter/v3 v3.8.0
	go.uber.org/ratelimit v0.2.0
)

require (
	github.com/andres-erbsen/clock v0.0.0-20160526145045-9e14626cd129 // indirect
	github.com/go-pkgz/expirable-cache v0.0.3 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/time v0.12.0 // indirect
)
