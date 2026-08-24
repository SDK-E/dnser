package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Format string

const (
	FormatText   Format = "text"
	FormatJSON   Format = "json"
	FormatNDJSON Format = "ndjson"
)

type Output struct {
	Stdout io.Writer
	Stderr io.Writer
	Format Format
	Fields []string
	isTTY  bool
}

func NewOutput(stdout, stderr *os.File, formatFlag, fieldsFlag string) *Output {
	return NewOutputWriter(stdout, stderr, formatFlag, fieldsFlag)
}

func NewOutputWriter(stdout, stderr io.Writer, formatFlag, fieldsFlag string) *Output {
	o := &Output{Stdout: stdout, Stderr: stderr}
	if f, ok := stdout.(*os.File); ok {
		o.isTTY = IsTTY(f)
	}
	switch {
	case formatFlag != "":
		o.Format = Format(formatFlag)
	case o.isTTY:
		o.Format = FormatText
	default:
		o.Format = FormatJSON
	}
	if fieldsFlag != "" {
		for _, f := range strings.Split(fieldsFlag, ",") {
			if f = strings.TrimSpace(f); f != "" {
				o.Fields = append(o.Fields, f)
			}
		}
	}
	return o
}

func (o *Output) Emit(data any) error {
	b, err := o.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(o.Stdout, string(b))
	return err
}

func (o *Output) Marshal(data any) ([]byte, error) {
	pruned := pruneFields(data, o.Fields)
	switch o.Format {
	case FormatJSON:
		b, err := json.MarshalIndent(pruned, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode json: %w", err)
		}
		return b, nil
	case FormatNDJSON:
		b, err := json.Marshal(pruned)
		if err != nil {
			return nil, fmt.Errorf("encode ndjson: %w", err)
		}
		return b, nil
	default:
		return textRender(pruned), nil
	}
}

func (o *Output) EmitList(items []any) error {
	switch o.Format {
	case FormatNDJSON:
		w := o.Stdout
		for _, item := range items {
			b, err := json.Marshal(pruneFields(item, o.Fields))
			if err != nil {
				return fmt.Errorf("encode ndjson line: %w", err)
			}
			if _, err := fmt.Fprintln(w, string(b)); err != nil {
				return err
			}
		}
		return nil
	default:
		return o.Emit(items)
	}
}

func pruneFields(data any, fields []string) any {
	if len(fields) == 0 {
		return data
	}
	m, ok := data.(map[string]any)
	if !ok {
		b, err := json.Marshal(data)
		if err != nil {
			return data
		}
		var generic map[string]any
		if json.Unmarshal(b, &generic) != nil {
			return data
		}
		m = generic
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := m[f]; ok {
			out[f] = v
		}
	}
	return out
}

func textRender(v any) []byte {
	switch t := v.(type) {
	case string:
		return []byte(t)
	default:
		b, _ := json.Marshal(t)
		return b
	}
}

func IsTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

var _ = os.Stdout
