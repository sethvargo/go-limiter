module github.com/sethvargo/go-limiter/benchmarks

go 1.25

replace github.com/sethvargo/go-limiter => ../

require (
	github.com/didip/tollbooth/v6 v6.1.2
	github.com/gomodule/redigo v1.9.3
	github.com/sethvargo/go-limiter v0.6.0
	github.com/sethvargo/go-redisstore v0.3.0
	github.com/throttled/throttled v2.2.5+incompatible
	github.com/ulule/limiter/v3 v3.11.2
	go.uber.org/ratelimit v0.3.1
)

require (
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/go-pkgz/expirable-cache v1.0.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/time v0.14.0 // indirect
)
