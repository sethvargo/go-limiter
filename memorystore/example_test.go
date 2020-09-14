package memorystore_test

import (
	"context"
	"log"
	"time"

	"github.com/sethvargo/go-limiter/memorystore"
)

func ExampleNew() {
	ctx := context.Background()

	store, err := memorystore.New(&memorystore.Config{
		Tokens:   15,
		Interval: time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close(ctx)

	limit, remaining, reset, ok, err := store.Take(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	_, _, _, _ = limit, remaining, reset, ok
}
