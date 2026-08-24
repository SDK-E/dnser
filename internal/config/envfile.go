package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func LoadEnvFiles(paths []string) (map[string]string, error) {
	merged := map[string]string{}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("env_file %s: %w", p, err)
		}
		parsed, err := godotenv.Parse(strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("parse env_file %s: %w", p, err)
		}
		for k, v := range parsed {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}
	return merged, nil
}

func MergeEnv(envFiles map[string]string, manifestEnv map[string]string) (map[string]string, map[string]string) {
	values := map[string]string{}
	sources := map[string]string{}
	for k, v := range envFiles {
		values[k] = v
		sources[k] = "env_file"
	}
	for k, v := range manifestEnv {
		values[k] = v
		sources[k] = SourceManifest.String()
	}
	return values, sources
}

const redactedPlaceholder = "••••••"

var nonSecretKeys = []string{
	"PORT", "DNSER_DOMAIN", "NODE_ENV", "APP_ENV", "RAILS_ENV",
	"NODE_OPTIONS", "PHP_CLI_SERVER_WORKERS", "GO_ENV", "DEBUG",
}

func RedactEnv(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		if isNonSecretKey(k) {
			out[k] = v
			continue
		}
		out[k] = redactedPlaceholder
	}
	return out
}

func RedactValue(key, value string) string {
	if isNonSecretKey(key) {
		return value
	}
	return redactedPlaceholder
}

func isNonSecretKey(key string) bool {
	for _, safe := range nonSecretKeys {
		if strings.EqualFold(key, safe) {
			return true
		}
	}
	return false
}
