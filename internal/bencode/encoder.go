package bencode

import (
	"bufio"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

// Encoder handles bencode encoding operations.
// Bencode encoding supports integers, strings, lists, and dictionaries.
type Encoder struct {
	writer *bufio.Writer
}

// NewEncoder creates a new bencode encoder that writes to the given writer.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		writer: bufio.NewWriter(w),
	}
}

// Encode converts a Go value to bencode format and writes it.
// Supports int, string, []byte, slices, arrays, and maps.
func (e *Encoder) Encode(value interface{}) error {
	err := e.encodeValue(value)
	if err != nil {
		return err
	}
	return e.writer.Flush()
}

// encodeValue handles the main encoding logic based on the value type.
func (e *Encoder) encodeValue(value interface{}) error {
	if value == nil {
		return fmt.Errorf("cannot encode nil value")
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.encodeInteger(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.encodeInteger(int64(v.Uint()))
	case reflect.String:
		return e.encodeString([]byte(v.String()))
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			// []byte
			return e.encodeString(v.Bytes())
		}
		return e.encodeList(value)
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			// [N]byte
			bytes := make([]byte, v.Len())
			for i := 0; i < v.Len(); i++ {
				bytes[i] = byte(v.Index(i).Uint())
			}
			return e.encodeString(bytes)
		}
		return e.encodeList(value)
	case reflect.Map:
		return e.encodeDictionary(value)
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
}

// encodeInteger writes an integer in bencode format: i<number>e
func (e *Encoder) encodeInteger(value int64) error {
	_, err := e.writer.WriteString("i" + strconv.FormatInt(value, 10) + "e")
	return err
}

// encodeString writes a string in bencode format: <length>:<data>
func (e *Encoder) encodeString(value []byte) error {
	length := strconv.Itoa(len(value))
	_, err := e.writer.WriteString(length + ":")
	if err != nil {
		return err
	}
	_, err = e.writer.Write(value)
	return err
}

// encodeList writes a list in bencode format: l<items>e
func (e *Encoder) encodeList(value interface{}) error {
	v := reflect.ValueOf(value)

	_, err := e.writer.WriteString("l")
	if err != nil {
		return err
	}

	for i := 0; i < v.Len(); i++ {
		err = e.encodeValue(v.Index(i).Interface())
		if err != nil {
			return err
		}
	}

	_, err = e.writer.WriteString("e")
	return err
}

// encodeDictionary writes a dictionary in bencode format: d<key><value>...e
// Keys are automatically sorted for compliance with bencode specification.
func (e *Encoder) encodeDictionary(value interface{}) error {
	v := reflect.ValueOf(value)

	_, err := e.writer.WriteString("d")
	if err != nil {
		return err
	}

	// Get sorted keys
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})

	for _, key := range keys {
		// Encode key (must be string)
		if key.Kind() != reflect.String {
			return fmt.Errorf("dictionary keys must be strings, got %T", key.Interface())
		}

		err = e.encodeString([]byte(key.String()))
		if err != nil {
			return err
		}

		// Encode value
		err = e.encodeValue(v.MapIndex(key).Interface())
		if err != nil {
			return err
		}
	}

	_, err = e.writer.WriteString("e")
	return err
}
