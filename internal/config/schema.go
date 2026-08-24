package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

func GenerateManifestSchema() ([]byte, error) {
	r := &jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
		ExpandedStruct:             true,
	}
	s := r.Reflect(&Manifest{})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}
	return buf.Bytes(), nil
}
