package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sethvargo/go-limiter/memorystore"
)

func main() {
	store, err := memorystore.New(&memorystore.Config{
		// Number of tokens allowed per interval.
		Tokens: 15,

		// Interval until tokens reset.
		Interval: time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// key is the unique value upon which you want to rate limit, like an IP or
	// MAC address.
	key := "127.0.0.1"

	for i := 0; i < 20; i++ {
		// Take a token.
		tokens, remaining, reset, ok, err := store.Take(ctx, key)
		fmt.Println(tokens, remaining, reset, ok, err)
	}

}
