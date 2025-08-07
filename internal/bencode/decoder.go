package bencode

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// BencodeDecoder decodes bencoded data
type BencodeDecoder struct {
	reader *bufio.Reader
}

// NewDecoder creates a new bencode decoder
func NewDecoder(r io.Reader) *BencodeDecoder {
	return &BencodeDecoder{
		reader: bufio.NewReader(r),
	}
}

// Decode decodes bencoded data into an interface{}
func (d *BencodeDecoder) Decode() (interface{}, error) {
	return d.decodeValue()
}

func (d *BencodeDecoder) decodeValue() (interface{}, error) {
	b, err := d.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read byte: %w", err)
	}

	switch {
	case b == 'i':
		// Integer
		return d.decodeInteger()
	case b == 'l':
		// List
		return d.decodeList()
	case b == 'd':
		// Dictionary
		return d.decodeDictionary()
	case b >= '0' && b <= '9':
		// String - unread the byte and decode
		err = d.reader.UnreadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %w", err)
		}
		return d.decodeString()
	default:
		return nil, fmt.Errorf("invalid bencode data: unexpected byte %c", b)
	}
}

func (d *BencodeDecoder) decodeInteger() (int64, error) {
	var result []byte
	
	for {
		b, err := d.reader.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("failed to read integer: %w", err)
		}
		
		if b == 'e' {
			break
		}
		
		result = append(result, b)
	}
	
	if len(result) == 0 {
		return 0, errors.New("empty integer")
	}
	
	// Check for leading zeros (except for "0")
	if len(result) > 1 && result[0] == '0' {
		return 0, errors.New("invalid integer: leading zero")
	}
	
	// Check for negative zero
	if len(result) == 2 && result[0] == '-' && result[1] == '0' {
		return 0, errors.New("invalid integer: negative zero")
	}
	
	num, err := strconv.ParseInt(string(result), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse integer: %w", err)
	}
	
	return num, nil
}

func (d *BencodeDecoder) decodeString() ([]byte, error) {
	var lengthBytes []byte
	
	// Read length until ':'
	for {
		b, err := d.reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read string length: %w", err)
		}
		
		if b == ':' {
			break
		}
		
		if b < '0' || b > '9' {
			return nil, fmt.Errorf("invalid string length character: %c", b)
		}
		
		lengthBytes = append(lengthBytes, b)
	}
	
	if len(lengthBytes) == 0 {
		return nil, errors.New("empty string length")
	}
	
	length, err := strconv.ParseInt(string(lengthBytes), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse string length: %w", err)
	}
	
	if length < 0 {
		return nil, errors.New("negative string length")
	}
	
	// Read the string data
	data := make([]byte, length)
	_, err = io.ReadFull(d.reader, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read string data: %w", err)
	}
	
	return data, nil
}

func (d *BencodeDecoder) decodeList() ([]interface{}, error) {
	var list []interface{}
	
	for {
		// Check for end marker
		b, err := d.reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read list: %w", err)
		}
		
		if b == 'e' {
			break
		}
		
		// Unread the byte and decode the value
		err = d.reader.UnreadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %w", err)
		}
		
		value, err := d.decodeValue()
		if err != nil {
			return nil, fmt.Errorf("failed to decode list element: %w", err)
		}
		
		list = append(list, value)
	}
	
	return list, nil
}

func (d *BencodeDecoder) decodeDictionary() (map[string]interface{}, error) {
	dict := make(map[string]interface{})
	var lastKey string
	
	for {
		// Check for end marker
		b, err := d.reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read dictionary: %w", err)
		}
		
		if b == 'e' {
			break
		}
		
		// Unread the byte and decode the key
		err = d.reader.UnreadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %w", err)
		}
		
		// Keys must be strings
		keyBytes, err := d.decodeString()
		if err != nil {
			return nil, fmt.Errorf("failed to decode dictionary key: %w", err)
		}
		
		key := string(keyBytes)
		
		// Check for proper ordering
		if key <= lastKey && lastKey != "" {
			return nil, fmt.Errorf("dictionary keys not in sorted order: %s <= %s", key, lastKey)
		}
		lastKey = key
		
		// Decode the value
		value, err := d.decodeValue()
		if err != nil {
			return nil, fmt.Errorf("failed to decode dictionary value for key %s: %w", key, err)
		}
		
		dict[key] = value
	}
	
	return dict, nil
}
