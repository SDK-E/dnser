package dnsl

import "time"

const (
	ReadTimeout    = 2 * time.Second
	WriteTimeout   = 2 * time.Second
	ForwardTimeout = 3 * time.Second
	ProbeTimeout   = 2 * time.Second
	StartTimeout   = 5 * time.Second

	readTimeout    = ReadTimeout
	writeTimeout   = WriteTimeout
	forwardTimeout = ForwardTimeout
	probeTimeout   = ProbeTimeout
)
