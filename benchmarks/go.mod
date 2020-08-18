module github.com/sethvargo/go-limiter/benchmarks

go 1.14

require (
	github.com/didip/tollbooth/v6 v6.0.1
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/sethvargo/go-limiter v0.3.2-0.20200818185929-f9ab9053588b
	github.com/sethvargo/go-redisstore v0.1.0
	github.com/throttled/throttled v2.2.4+incompatible
	github.com/ulule/limiter/v3 v3.5.0
	go.uber.org/atomic v1.6.0 // indirect
	go.uber.org/ratelimit v0.1.0
)

replace github.com/sethvargo/go-limiter => ../
