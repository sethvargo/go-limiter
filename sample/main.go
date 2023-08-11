package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sethvargo/go-limiter/daprstore"
)

func main() {
	store, err := daprstore.New(&daprstore.Config{
		// Number of tokens allowed per interval.
		Tokens: 1,

		// Interval until tokens reset.
		Interval: time.Second * 1,
	})

	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// key is the unique value upon which you want to rate limit, like an IP or
	// MAC address.
	key := "127.0.0.1"

	for i := 0; i < 50; i++ {
		// Take a token.
		tokens, remaining, reset, ok, err := store.Take(ctx, key)
		for (!ok && remaining == 0) || err != nil {
			if err != nil {
				break
			}
			// Rate limit exceeded.
			tokenAvailableTime := time.Unix(0, int64(reset))
			//fmt.Println("Rate limit exceeded - waiting until", tokenAvailableTime)
			time.Sleep(time.Until(tokenAvailableTime))
			tokens, remaining, reset, ok, err = store.Take(ctx, key)
		}
		fmt.Println(i, tokens, remaining, reset, ok, err)
	}
}
