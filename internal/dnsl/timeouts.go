package dnsl

import "time"

const (
	readTimeout    = 2 * time.Second
	writeTimeout   = 2 * time.Second
	forwardTimeout = 3 * time.Second
	probeTimeout   = 2 * time.Second
)
