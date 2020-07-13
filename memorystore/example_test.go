package memorystore_test

import (
	"log"
	"time"

	"github.com/sethvargo/go-limiter/memorystore"
)

func ExampleNew() {
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   15,
		Interval: time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	limit, remaining, reset, ok := store.Take("my-key")
	_, _, _, _ = limit, remaining, reset, ok
}
