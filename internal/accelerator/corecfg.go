package accelerator

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

var ErrCoreConfigConflict = errors.New("Accelerator core.cfg has been modified outside Panel ownership")

var managedCoreKeys = []string{
	"MinidumpUrl",
	"MinidumpSymbolUrl",
	"MinidumpBinaryUrl",
	"MinidumpPresubmit",
	"MinidumpSymbolUpload",
	"MinidumpBinaryUpload",
}

type CoreConfigChange struct {
	Previous     string `json:"previous"`
	PreviousRaw  string `json:"previous_raw,omitempty"`
	Written      string `json:"written"`
	Present      bool   `json:"present"`
	InsertedText string `json:"inserted_text,omitempty"`
}

type coreToken struct {
	kind  byte
	value string
	start int
	end   int
}

type corePair struct {
	key    coreToken
	value  *coreToken
	object *coreObject
}

type coreObject struct {
	pairs    []corePair
	closePos int
}

type coreReplacement struct {
	start int
	end   int
	value string
}

func patchCoreConfig(original []byte, panelPort int, token string) ([]byte, map[string]CoreConfigChange, error) {
	if panelPort < 1 || panelPort > 65535 {
		return nil, nil, errors.New("invalid Panel port")
	}
	document, err := parseCoreConfig(original)
	if err != nil {
		return nil, nil, err
	}
	core, err := findCoreObject(document)
	if err != nil {
		return nil, nil, err
	}
	tokenValue := url.QueryEscape(token)
	desired := map[string]string{
		"MinidumpUrl":          fmt.Sprintf("http://127.0.0.1:%d/submit?token=%s", panelPort, tokenValue),
		"MinidumpSymbolUrl":    fmt.Sprintf("http://127.0.0.1:%d/symbols/submit?token=%s", panelPort, tokenValue),
		"MinidumpBinaryUrl":    fmt.Sprintf("http://127.0.0.1:%d/binary/submit?token=%s", panelPort, tokenValue),
		"MinidumpPresubmit":    "yes",
		"MinidumpSymbolUpload": "3",
		"MinidumpBinaryUpload": "yes",
	}
	changes := make(map[string]CoreConfigChange, len(managedCoreKeys))
	replacements := make([]coreReplacement, 0, len(managedCoreKeys)+1)
	missing := make([]string, 0)
	for _, key := range managedCoreKeys {
		matches := make([]corePair, 0, 1)
		for _, pair := range core.pairs {
			if pair.key.value == key {
				matches = append(matches, pair)
			}
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("duplicate managed core.cfg key %q", key)
		}
		written := desired[key]
		if len(matches) == 0 {
			missing = append(missing, key)
			changes[key] = CoreConfigChange{Written: written}
			continue
		}
		pair := matches[0]
		if pair.value == nil {
			return nil, nil, fmt.Errorf("managed core.cfg key %q is not a scalar", key)
		}
		changes[key] = CoreConfigChange{
			Previous: stringValue(pair.value.value), PreviousRaw: string(original[pair.value.start:pair.value.end]),
			Written: written, Present: true,
		}
		replacements = append(replacements, coreReplacement{start: pair.value.start, end: pair.value.end, value: quoteCoreValue(written)})
	}
	if len(missing) > 0 {
		indent := coreIndent(original, core)
		prefix := ""
		if core.closePos == 0 || original[core.closePos-1] != '\n' {
			prefix = "\n"
		}
		for index, key := range missing {
			line := indent + quoteCoreValue(key) + " " + quoteCoreValue(desired[key]) + "\n"
			inserted := line
			if index == 0 {
				inserted = prefix + inserted
			}
			change := changes[key]
			change.InsertedText = inserted
			changes[key] = change
			prefix += line
		}
		block := ""
		for _, key := range missing {
			block += changes[key].InsertedText
		}
		replacements = append(replacements, coreReplacement{start: core.closePos, end: core.closePos, value: block})
	}
	return applyCoreReplacements(original, replacements), changes, nil
}

func restoreCoreConfig(current []byte, changes map[string]CoreConfigChange) ([]byte, error) {
	document, err := parseCoreConfig(current)
	if err != nil {
		return nil, ErrCoreConfigConflict
	}
	core, err := findCoreObject(document)
	if err != nil {
		return nil, ErrCoreConfigConflict
	}
	replacements := make([]coreReplacement, 0, len(changes))
	for key, change := range changes {
		matches := make([]corePair, 0, 1)
		for _, pair := range core.pairs {
			if pair.key.value == key {
				matches = append(matches, pair)
			}
		}
		if len(matches) != 1 || matches[0].value == nil || matches[0].value.value != change.Written {
			return nil, ErrCoreConfigConflict
		}
		pair := matches[0]
		if change.Present {
			raw := change.PreviousRaw
			if raw == "" {
				raw = quoteCoreValue(change.Previous)
			}
			replacements = append(replacements, coreReplacement{start: pair.value.start, end: pair.value.end, value: raw})
			continue
		}
		if change.InsertedText == "" {
			return nil, ErrCoreConfigConflict
		}
		start := bytes.Index(current, []byte(change.InsertedText))
		if start < 0 {
			return nil, ErrCoreConfigConflict
		}
		replacements = append(replacements, coreReplacement{start: start, end: start + len(change.InsertedText)})
	}
	return applyCoreReplacements(current, replacements), nil
}

func applyCoreReplacements(original []byte, replacements []coreReplacement) []byte {
	sort.SliceStable(replacements, func(i, j int) bool {
		return replacements[i].start > replacements[j].start
	})
	result := append([]byte(nil), original...)
	for _, replacement := range replacements {
		result = append(result[:replacement.start], append([]byte(replacement.value), result[replacement.end:]...)...)
	}
	return result
}

func parseCoreConfig(raw []byte) (coreObject, error) {
	tokens, err := lexCoreConfig(raw)
	if err != nil {
		return coreObject{}, err
	}
	root, position, err := parseCoreObject(tokens, 0, false)
	if err != nil || position != len(tokens) {
		return coreObject{}, errors.New("malformed core.cfg nesting")
	}
	return root, nil
}

func findCoreObject(root coreObject) (coreObject, error) {
	var found *coreObject
	for _, pair := range root.pairs {
		if pair.key.value != "Core" {
			continue
		}
		if pair.object == nil || found != nil {
			return coreObject{}, errors.New("core.cfg must contain exactly one Core object")
		}
		copy := *pair.object
		found = &copy
	}
	if found == nil {
		return coreObject{}, errors.New("core.cfg Core object is missing")
	}
	return *found, nil
}

func parseCoreObject(tokens []coreToken, position int, expectClose bool) (coreObject, int, error) {
	object := coreObject{}
	if expectClose {
		if position >= len(tokens) || tokens[position].kind != '{' {
			return coreObject{}, position, errors.New("missing core.cfg object opening brace")
		}
		position++
	}
	for position < len(tokens) {
		if tokens[position].kind == '}' {
			if !expectClose {
				return coreObject{}, position, errors.New("unexpected core.cfg closing brace")
			}
			object.closePos = tokens[position].start
			return object, position + 1, nil
		}
		key := tokens[position]
		if key.kind != 'v' {
			return coreObject{}, position, errors.New("core.cfg key expected")
		}
		position++
		if position >= len(tokens) {
			return coreObject{}, position, errors.New("core.cfg value missing")
		}
		if tokens[position].kind == '{' {
			child, next, err := parseCoreObject(tokens, position, true)
			if err != nil {
				return coreObject{}, position, err
			}
			object.pairs = append(object.pairs, corePair{key: key, object: &child})
			position = next
			continue
		}
		if tokens[position].kind != 'v' {
			return coreObject{}, position, errors.New("core.cfg scalar value expected")
		}
		value := tokens[position]
		position++
		object.pairs = append(object.pairs, corePair{key: key, value: &value})
	}
	if expectClose {
		return coreObject{}, position, errors.New("core.cfg object is not closed")
	}
	return object, position, nil
}

func lexCoreConfig(raw []byte) ([]coreToken, error) {
	tokens := make([]coreToken, 0)
	for position := 0; position < len(raw); {
		if raw[position] == ' ' || raw[position] == '\t' || raw[position] == '\r' || raw[position] == '\n' {
			position++
			continue
		}
		if raw[position] == '/' && position+1 < len(raw) && raw[position+1] == '/' {
			position += 2
			for position < len(raw) && raw[position] != '\n' {
				position++
			}
			continue
		}
		if raw[position] == '/' && position+1 < len(raw) && raw[position+1] == '*' {
			end := bytes.Index(raw[position+2:], []byte("*/"))
			if end < 0 {
				return nil, errors.New("unterminated core.cfg comment")
			}
			position += end + 4
			continue
		}
		if raw[position] == '{' || raw[position] == '}' {
			tokens = append(tokens, coreToken{kind: raw[position], start: position, end: position + 1})
			position++
			continue
		}
		start := position
		if raw[position] == '"' {
			position++
			escaped := false
			closed := false
			for position < len(raw) {
				char := raw[position]
				position++
				if escaped {
					escaped = false
					continue
				}
				if char == '\\' {
					escaped = true
					continue
				}
				if char == '"' {
					closed = true
					break
				}
			}
			if !closed {
				return nil, errors.New("unterminated core.cfg string")
			}
			rawValue := string(raw[start:position])
			value, err := strconv.Unquote(rawValue)
			if err != nil {
				return nil, errors.New("invalid core.cfg quoted value")
			}
			tokens = append(tokens, coreToken{kind: 'v', value: value, start: start, end: position})
			continue
		}
		for position < len(raw) && raw[position] != ' ' && raw[position] != '\t' && raw[position] != '\r' && raw[position] != '\n' && raw[position] != '{' && raw[position] != '}' {
			position++
		}
		if position == start {
			return nil, errors.New("invalid core.cfg token")
		}
		tokens = append(tokens, coreToken{kind: 'v', value: string(raw[start:position]), start: start, end: position})
	}
	return tokens, nil
}

func coreIndent(raw []byte, object coreObject) string {
	if len(object.pairs) > 0 {
		start := object.pairs[0].key.start
		lineStart := bytes.LastIndexByte(raw[:start], '\n') + 1
		indent := raw[lineStart:start]
		for _, char := range indent {
			if char != ' ' && char != '\t' && char != '\r' {
				return "\t"
			}
		}
		return string(indent)
	}
	return "\t"
}

func quoteCoreValue(value string) string {
	return strconv.Quote(value)
}

func stringValue(value string) string {
	return value
}
