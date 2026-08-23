package config

func DefaultPorts() Ports {
	return Ports{DNS: 53, HTTP: 80, HTTPS: 443, UI: 4500}
}

func DefaultUpstreams() []string {
	return []string{"9.9.9.9", "149.112.112.112", "1.1.1.1"}
}

func DefaultSettings() Settings {
	return Settings{
		TLD:       "test",
		Bind:      "127.0.0.1",
		Upstreams: DefaultUpstreams(),
		Autostart: true,
		Ports:     DefaultPorts(),
	}
}

func Default() Config {
	return Config{
		Version:  CurrentVersion,
		Settings: DefaultSettings(),
		Projects: nil,
	}
}
