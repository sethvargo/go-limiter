package daprstore_test

import (
	"context"
	"log"
	"time"

	"github.com/sethvargo/go-limiter/daprstore"
)

func ExampleNew() {
	ctx := context.Background()

	store, err := daprstore.New(&daprstore.Config{
		Tokens:         15,
		Interval:       time.Minute,
		StateStoreName: "statestore",
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
