module github.com/sethvargo/go-limiter/benchmarks

go 1.14

replace github.com/sethvargo/go-limiter => ../

require (
	github.com/didip/tollbooth/v6 v6.1.1
	github.com/gomodule/redigo v1.8.5
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/sethvargo/go-limiter v0.6.0
	github.com/sethvargo/go-redisstore v0.3.0
	github.com/throttled/throttled v2.2.5+incompatible
	github.com/ulule/limiter/v3 v3.8.0
	go.uber.org/ratelimit v0.2.0
)
