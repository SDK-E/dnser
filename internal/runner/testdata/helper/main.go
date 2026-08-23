package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if os.Getenv("HELPER_MODE") == "exit" {
		fmt.Fprintln(os.Stderr, "simulated crash")
		os.Exit(1)
	}
	addr := "127.0.0.1:" + os.Getenv("PORT")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "helper-up")
	})
	fmt.Println("helper listening on", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
}
