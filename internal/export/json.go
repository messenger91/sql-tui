package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

// JSONString renders a result as an indented JSON array of objects. Column
// order is preserved (encoding/json sorts map keys, so this uses an ordered
// object) and duplicate column names are kept.
func JSONString(result db.QueryResult) (string, error) {
	var builder strings.Builder
	if err := WriteJSON(&builder, result); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// WriteJSON writes the indented JSON array plus a trailing newline.
func WriteJSON(w io.Writer, result db.QueryResult) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonObjects(result)); err != nil {
		return fmt.Errorf("export: encode json: %w", err)
	}
	return nil
}

type jsonField struct {
	name  string
	value any
}

type jsonObject []jsonField

func (o jsonObject) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for i, field := range o {
		if i > 0 {
			buffer.WriteByte(',')
		}
		key, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		buffer.Write(key)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func jsonObjects(result db.QueryResult) []jsonObject {
	objects := make([]jsonObject, 0, len(result.Rows))
	for _, row := range result.Rows {
		object := make(jsonObject, 0, len(result.Columns))
		for i, name := range result.Columns {
			var value any
			if i < len(row) {
				value = JSONValue(row[i])
			}
			object = append(object, jsonField{name: name, value: value})
		}
		objects = append(objects, object)
	}
	return objects
}

// JSONValue keeps JSON-native types as-is and converts driver-specific values
// (binary blobs, times, unknown types) into strings.
func JSONValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v
	case []byte:
		return FormatValue(v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return FormatValue(v)
	}
}
