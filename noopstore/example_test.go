package noopstore_test

import (
	"log"

	"github.com/sethvargo/go-limiter/noopstore"
)

func ExampleNew() {
	store, err := noopstore.New()
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	limit, remaining, reset, ok := store.Take("my-key")
	_, _, _, _ = limit, remaining, reset, ok
}
