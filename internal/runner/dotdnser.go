package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const dotDnserFile = ".dnser.yaml"

type LinkOverride struct {
	Command string
}

type DotDnser struct {
	Command  string
	Services []DotService
	Routes   []DotRoute
}

type DotService struct {
	Name      string
	Type      string
	Command   string
	Host      string
	Port      int
	Transport string
	DNS       *bool
}

type DotRoute struct {
	Host       string
	Backends   []string
	TCP        bool
	UDP        bool
	HTTPS      bool
	ForceHTTPS bool
	Listen     int
	Paths      []string
}

func ReadLinkOverride(dir string) (LinkOverride, bool) {
	doc, err := ParseDotDnserDir(dir)
	if err != nil || doc.Command == "" {
		return LinkOverride{}, false
	}
	return LinkOverride{Command: doc.Command}, true
}

func ReadLinkOverrideFromSource(src string) (LinkOverride, bool) {
	doc, err := ParseDotDnser(src)
	if err != nil || doc.Command == "" {
		return LinkOverride{}, false
	}
	return LinkOverride{Command: doc.Command}, true
}

func ParseDotDnserDir(dir string) (*DotDnser, error) {
	return ParseDotDnserFile(filepath.Join(dir, dotDnserFile))
}

func ParseDotDnserFile(path string) (*DotDnser, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := ParseDotDnser(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

func ParseDotDnser(src string) (*DotDnser, error) {
	lines, err := scanLines(src)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return &DotDnser{}, nil
	}
	tree, next := parseBlock(lines, 0, minIndent(lines))
	if next != len(lines) {
		return nil, fmt.Errorf("line %d: inconsistent indentation", lines[next].num)
	}
	root, ok := tree.(map[string]mapItem)
	if !ok {
		return nil, fmt.Errorf("expected top-level mappings")
	}
	doc := &DotDnser{}
	if v, ok := root["command"]; ok {
		doc.Command = scalarString(v.value)
	}
	if v, ok := root["services"]; ok {
		svcs, err := parseServices(v.value)
		if err != nil {
			return nil, err
		}
		doc.Services = svcs
	}
	if v, ok := root["routes"]; ok {
		rts, err := parseRoutes(v.value)
		if err != nil {
			return nil, err
		}
		doc.Routes = rts
	}
	return doc, nil
}

type rawLine struct {
	num    int
	indent int
	text   string
}

type node struct {
	key    string
	value  any
	inline bool
}

type mapItem node

type listItem struct {
	value any
	num   int
}

func scanLines(src string) ([]rawLine, error) {
	var out []rawLine
	for i, raw := range strings.Split(src, "\n") {
		raw = strings.TrimRight(raw, "\r")
		stripped := stripComment(raw)
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		indent := 0
		for _, r := range stripped {
			switch r {
			case ' ':
				indent++
			case '\t':
				return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
			default:
				out = append(out, rawLine{num: i + 1, indent: indent, text: trimmed})
			}
			if r != ' ' && r != '\t' {
				break
			}
		}
	}
	return out, nil
}

func stripComment(line string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
				return line[:i]
			}
		}
	}
	return line
}

func minIndent(lines []rawLine) int {
	m := -1
	for _, l := range lines {
		if m == -1 || l.indent < m {
			m = l.indent
		}
	}
	return m
}

func parseBlock(lines []rawLine, i, indent int) (any, int) {
	if i >= len(lines) {
		return nil, i
	}
	if strings.HasPrefix(lines[i].text, "- ") || lines[i].text == "-" {
		return parseList(lines, i, indent)
	}
	return parseMapping(lines, i, indent)
}

func parseMapping(lines []rawLine, i, indent int) (any, int) {
	m := map[string]mapItem{}
	for i < len(lines) {
		line := lines[i]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, i
		}
		if strings.HasPrefix(line.text, "- ") || line.text == "-" {
			break
		}
		key, rest, ok := splitKey(line)
		if !ok {
			return nil, i
		}
		i++
		if rest != "" {
			m[key] = mapItem{key: key, value: rest, inline: true}
			continue
		}
		if i < len(lines) && lines[i].indent > indent {
			if isScalarBlock(lines[i]) {
				base := lines[i].indent
				var parts []string
				for i < len(lines) && lines[i].indent >= base {
					parts = append(parts, strings.TrimSpace(lines[i].text))
					i++
				}
				m[key] = mapItem{key: key, value: strings.Join(parts, " "), inline: true}
				continue
			}
			child, next := parseBlock(lines, i, lines[i].indent)
			i = next
			m[key] = mapItem{key: key, value: child}
			continue
		}
		if i < len(lines) && lines[i].indent == indent && (strings.HasPrefix(lines[i].text, "- ") || lines[i].text == "-") {
			child, next := parseList(lines, i, indent)
			i = next
			m[key] = mapItem{key: key, value: child}
			continue
		}
		m[key] = mapItem{key: key, value: nil}
	}
	return m, i
}

func isScalarBlock(line rawLine) bool {
	if strings.HasPrefix(line.text, "- ") || line.text == "-" {
		return false
	}
	_, _, ok := splitKeyText(line.text)
	return !ok
}

func parseList(lines []rawLine, i, indent int) (any, int) {
	var out []listItem
	for i < len(lines) {
		line := lines[i]
		if line.indent < indent || !strings.HasPrefix(line.text, "- ") && line.text != "-" {
			break
		}
		if line.indent != indent {
			return nil, i
		}
		startNum := line.num
		rest := ""
		if line.text != "-" {
			rest = strings.TrimSpace(strings.TrimPrefix(line.text, "-"))
		}
		i++
		if rest == "" {
			if i < len(lines) && lines[i].indent > indent {
				child, next := parseBlock(lines, i, lines[i].indent)
				i = next
				out = append(out, listItem{value: child, num: startNum})
				continue
			}
			out = append(out, listItem{value: nil, num: startNum})
			continue
		}
		if _, _, hasKey := splitKeyText(rest); hasKey {
			virtualIndent := line.indent + (len(line.text) - len(rest))
			virt := make([]rawLine, 0, 4)
			virt = append(virt, rawLine{num: line.num, indent: virtualIndent, text: rest})
			for i < len(lines) && lines[i].indent >= virtualIndent {
				virt = append(virt, lines[i])
				i++
			}
			child, _ := parseBlock(virt, 0, virtualIndent)
			out = append(out, listItem{value: child, num: startNum})
			continue
		}
		out = append(out, listItem{value: rest, num: startNum})
	}
	return out, i
}

func splitKey(line rawLine) (string, string, bool) {
	return splitKeyText(line.text)
}

func splitKeyText(text string) (string, string, bool) {
	inSingle, inDouble := false, false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ':':
			if inSingle || inDouble {
				continue
			}
			if i+1 < len(text) && text[i+1] != ' ' {
				continue
			}
			key := unquote(strings.TrimSpace(text[:i]))
			rest := strings.TrimSpace(text[i+1:])
			return key, rest, true
		}
	}
	if strings.HasSuffix(text, ":") && !strings.Contains(text[:len(text)-1], "\"'") {
		return unquote(strings.TrimSpace(strings.TrimSuffix(text, ":"))), "", true
	}
	return "", "", false
}

func unquote(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'' {
			inner := s[1 : len(s)-1]
			if s[0] == '"' {
				inner = strings.ReplaceAll(inner, `\"`, `"`)
				inner = strings.ReplaceAll(inner, `\\`, `\`)
			} else {
				inner = strings.ReplaceAll(inner, `''`, `'`)
			}
			return inner
		}
	}
	return s
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return unquote(t)
	case map[string]mapItem:
		return ""
	case []listItem:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func scalarInt(v any, where string) (int, error) {
	s := scalarString(v)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not an integer", where, s)
	}
	return n, nil
}

func scalarBool(v any, where string) (bool, error) {
	s := strings.ToLower(scalarString(v))
	switch s {
	case "":
		return false, nil
	case "true", "yes", "on":
		return true, nil
	case "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s: %q is not a boolean", where, s)
	}
}

func stringSlice(v any, where string) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		s := unquote(t)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			return splitInlineList(s[1 : len(s)-1]), nil
		}
		if s == "" {
			return nil, nil
		}
		return []string{s}, nil
	case []listItem:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, scalarString(item.value))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s: unsupported value", where)
	}
}

func splitInlineList(s string) []string {
	var out []string
	depth := 0
	cur := strings.Builder{}
	inSingle, inDouble := false, false
	flush := func() {
		field := strings.TrimSpace(cur.String())
		if field != "" {
			out = append(out, unquote(field))
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == '[' && !inSingle && !inDouble:
			depth++
		case r == ']' && !inSingle && !inDouble:
			depth--
		case r == ',' && depth == 0 && !inSingle && !inDouble:
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

func parseServices(v any) ([]DotService, error) {
	var out []DotService
	appendService := func(name string, fields map[string]mapItem, num int) error {
		if name == "" {
			name = scalarString(fieldsValue(fields, "name"))
		}
		if name == "" {
			return fmt.Errorf("line %d: service entry missing name", num)
		}
		svc := DotService{Name: name, Type: scalarString(fieldsValue(fields, "type")),
			Command:   scalarString(fieldsValue(fields, "command")),
			Host:      scalarString(fieldsValue(fields, "host")),
			Transport: strings.ToLower(scalarString(fieldsValue(fields, "transport")))}
		port, err := scalarInt(fieldsValue(fields, "port"), fmt.Sprintf("line %d: service %s.port", num, name))
		if err != nil {
			return err
		}
		svc.Port = port
		if raw, ok := fields["dns"]; ok {
			b, err := scalarBool(raw.value, fmt.Sprintf("line %d: service %s.dns", num, name))
			if err != nil {
				return err
			}
			svc.DNS = &b
		} else {
			dnsDefault := true
			svc.DNS = &dnsDefault
		}
		out = append(out, svc)
		return nil
	}
	switch t := v.(type) {
	case map[string]mapItem:
		for _, item := range orderedItems(t) {
			fields, ok := item.value.(map[string]mapItem)
			if !ok {
				fields = map[string]mapItem{}
			}
			if err := appendService(item.key, fields, 0); err != nil {
				return nil, err
			}
		}
	case []listItem:
		for _, item := range t {
			fields, ok := item.value.(map[string]mapItem)
			if !ok {
				fields = map[string]mapItem{}
			}
			if err := appendService("", fields, item.num); err != nil {
				return nil, err
			}
		}
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("services: unsupported structure")
	}
	return out, nil
}

func fieldsValue(fields map[string]mapItem, key string) any {
	if f, ok := fields[key]; ok {
		return f.value
	}
	return nil
}

func orderedItems(m map[string]mapItem) []mapItem {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortKeys(keys)
	out := make([]mapItem, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func sortKeys(keys []string) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

func parseRoutes(v any) ([]DotRoute, error) {
	list, ok := v.([]listItem)
	if !ok {
		if v == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("routes: expected a list")
	}
	out := make([]DotRoute, 0, len(list))
	for _, item := range list {
		fields, ok := item.value.(map[string]mapItem)
		if !ok {
			return nil, fmt.Errorf("line %d: route entry must be a mapping", item.num)
		}
		rt := DotRoute{
			Host:       scalarString(fieldsValue(fields, "host")),
			Backends:   nil,
			TCP:        false,
			HTTPS:      false,
			ForceHTTPS: false,
		}
		var err error
		if rt.Backends, err = stringSlice(fieldsValue(fields, "backends"), fmt.Sprintf("line %d: route %s.backends", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.Paths, err = stringSlice(fieldsValue(fields, "paths"), fmt.Sprintf("line %d: route %s.paths", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.TCP, err = scalarBool(fieldsValue(fields, "tcp"), fmt.Sprintf("line %d: route %s.tcp", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.UDP, err = scalarBool(fieldsValue(fields, "udp"), fmt.Sprintf("line %d: route %s.udp", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.HTTPS, err = scalarBool(fieldsValue(fields, "https"), fmt.Sprintf("line %d: route %s.https", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.ForceHTTPS, err = scalarBool(fieldsValue(fields, "force_https"), fmt.Sprintf("line %d: route %s.force_https", item.num, rt.Host)); err != nil {
			return nil, err
		}
		if rt.Listen, err = scalarInt(fieldsValue(fields, "listen"), fmt.Sprintf("line %d: route %s.listen", item.num, rt.Host)); err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, nil
}
