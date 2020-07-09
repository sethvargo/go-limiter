package httplimit_test

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
)

var keyFunc httplimit.KeyFunc

func ExampleKeyFunc_custom() {
	// This is an example KeyFunc that rate limits using the value from the
	// X-API-Key header. Since this value is likely a secret, it is hashed before
	// passing along to the store.
	keyFunc = httplimit.KeyFunc(func(r *http.Request) (string, error) {
		dig := sha512.Sum512([]byte(r.Header.Get("X-Token")))
		return base64.StdEncoding.EncodeToString(dig[:]), nil
	})
	// middleware, err := httplimit.NewMiddleware(store, keyFunc)
}

func ExampleIPKeyFunc_headers() {
	keyFunc = httplimit.IPKeyFunc("X-Forwarded-For")
	// middleware, err := httplimit.NewMiddleware(store, keyFunc)
}

func ExampleNewMiddleware() {
	// Create a store that allows 30 requests per minute.
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   30,
		Interval: time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create the HTTP middleware from the store, keying by IP address.
	middleware, err := httplimit.NewMiddleware(store, httplimit.IPKeyFunc())
	if err != nil {
		log.Fatal(err)
	}

	doWork := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintf(w, "hello world")
	})

	// Wrap an individual handler (only rate limits this endpoint).
	mux1 := http.NewServeMux()
	mux1.Handle("/foo", middleware.Handle(doWork)) // rate limited
	mux1.Handle("/bar", doWork)                    // not rate limited
	_ = mux1

	// Or wrap the entire mux (rate limits all endpoints).
	mux2 := http.NewServeMux()
	mux2.Handle("/foo", doWork)
	mux2.Handle("/bar", doWork)
	router := middleware.Handle(mux2) // all endpoints are rate limited
	_ = router
}
