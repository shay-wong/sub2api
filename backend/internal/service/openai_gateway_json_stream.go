package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"unicode/utf8"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
)

const openAIJSONStringChunkSize = 32 << 10

func marshalOpenAIUpstreamJSONDiskBacked(v any) (*pkghttputil.MappedBody, []byte, error) {
	mapped, err := pkghttputil.NewMappedBody(func(w io.Writer) error {
		buffered := bufio.NewWriterSize(w, 64<<10)
		if err := (&openAIJSONStreamEncoder{w: buffered}).write(v); err != nil {
			return err
		}
		return buffered.Flush()
	})
	if err != nil {
		return nil, nil, err
	}
	return mapped, mapped.Bytes(), nil
}

type openAIJSONStreamEncoder struct {
	w       io.Writer
	scratch bytes.Buffer
}

func (e *openAIJSONStreamEncoder) write(v any) error {
	switch value := v.(type) {
	case map[string]any:
		return e.writeObject(value)
	case []any:
		return e.writeArray(value)
	case string:
		return e.writeString(value)
	default:
		return e.writeScalar(value)
	}
}

func (e *openAIJSONStreamEncoder) writeObject(value map[string]any) error {
	if _, err := io.WriteString(e.w, "{"); err != nil {
		return err
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		if i > 0 {
			if _, err := io.WriteString(e.w, ","); err != nil {
				return err
			}
		}
		if err := e.writeString(key); err != nil {
			return err
		}
		if _, err := io.WriteString(e.w, ":"); err != nil {
			return err
		}
		if err := e.write(value[key]); err != nil {
			return err
		}
	}
	_, err := io.WriteString(e.w, "}")
	return err
}

func (e *openAIJSONStreamEncoder) writeArray(value []any) error {
	if _, err := io.WriteString(e.w, "["); err != nil {
		return err
	}
	for i, item := range value {
		if i > 0 {
			if _, err := io.WriteString(e.w, ","); err != nil {
				return err
			}
		}
		if err := e.write(item); err != nil {
			return err
		}
	}
	_, err := io.WriteString(e.w, "]")
	return err
}

func (e *openAIJSONStreamEncoder) writeString(value string) error {
	if _, err := io.WriteString(e.w, `"`); err != nil {
		return err
	}
	for len(value) > 0 {
		end := min(len(value), openAIJSONStringChunkSize)
		if end < len(value) && !utf8.RuneStart(value[end]) {
			boundary := end
			for boundary > 0 && end-boundary < utf8.UTFMax && !utf8.RuneStart(value[boundary]) {
				boundary--
			}
			if utf8.RuneStart(value[boundary]) {
				end = boundary
			}
		}

		e.scratch.Reset()
		enc := json.NewEncoder(&e.scratch)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(value[:end]); err != nil {
			return err
		}
		encoded := e.scratch.Bytes()
		if len(encoded) < 3 || encoded[0] != '"' || encoded[len(encoded)-2] != '"' {
			return errors.New("encode JSON string chunk")
		}
		if _, err := e.w.Write(encoded[1 : len(encoded)-2]); err != nil {
			return err
		}
		value = value[end:]
	}
	_, err := io.WriteString(e.w, `"`)
	return err
}

func (e *openAIJSONStreamEncoder) writeScalar(value any) error {
	e.scratch.Reset()
	enc := json.NewEncoder(&e.scratch)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return err
	}
	encoded := e.scratch.Bytes()
	if len(encoded) > 0 && encoded[len(encoded)-1] == '\n' {
		encoded = encoded[:len(encoded)-1]
	}
	_, err := e.w.Write(encoded)
	return err
}
