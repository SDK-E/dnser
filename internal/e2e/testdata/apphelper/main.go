package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	addr := "127.0.0.1:" + os.Getenv("PORT")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "apphelper-up")
	})
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
