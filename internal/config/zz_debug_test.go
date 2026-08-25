package config

import (
	"fmt"
	"testing"
)

func TestDebugMailboxRoute(t *testing.T) {
	m, err := Decode([]byte("domain: mailbox.test\nport: 11090\nroutes:\n  - host: \"@\"\n    https: true\n    backends: [\"127.0.0.1:11090\"]\nservices:\n  smtp:\n    type: smtp\n    host: 127.0.0.1\n    port: 11025\n"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("routes=%+v\n", m.Routes)
	fmt.Printf("pername=%v\n", m.HTTPS)
}
