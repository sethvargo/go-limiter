package redisstore_test

import (
	"log"
	"net"
	"time"

	"github.com/sethvargo/go-limiter/redisstore"
)

func ExampleNew() {
	store, err := redisstore.New(&redisstore.Config{
		Tokens:          15,
		Interval:        time.Minute,
		InitialPoolSize: 32,
		MaxPoolSize:     128,
		AuthPassword:    "my-password",
		DialFunc: func() (net.Conn, error) {
			conn, err := net.Dial("tcp", "127.0.0.1:6379")
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer store.Stop()

	limit, remaining, reset, ok := store.Take("my-key")
	_, _, _, _ = limit, remaining, reset, ok
}
