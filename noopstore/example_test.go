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

	limit, remaining, reset, ok, err := store.Take("my-key")
	if err != nil {
		log.Fatal(err)
	}
	_, _, _, _ = limit, remaining, reset, ok
}
