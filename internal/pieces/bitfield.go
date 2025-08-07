package pieces

import (
	"fmt"
)

// Bitfield represents a bitfield for tracking pieces
type Bitfield struct {
	data []byte
	size int
}

// NewBitfield creates a new bitfield with the specified number of pieces
func NewBitfield(numPieces int) *Bitfield {
	numBytes := (numPieces + 7) / 8 // Round up to nearest byte
	return &Bitfield{
		data: make([]byte, numBytes),
		size: numPieces,
	}
}

// NewBitfieldFromBytes creates a bitfield from raw bytes
func NewBitfieldFromBytes(data []byte, numPieces int) *Bitfield {
	expectedBytes := (numPieces + 7) / 8
	if len(data) < expectedBytes {
		// Pad with zeros if too short
		padded := make([]byte, expectedBytes)
		copy(padded, data)
		data = padded
	}
	
	return &Bitfield{
		data: data,
		size: numPieces,
	}
}

// SetPiece marks a piece as available
func (bf *Bitfield) SetPiece(pieceIndex int) error {
	if pieceIndex < 0 || pieceIndex >= bf.size {
		return fmt.Errorf("piece index %d out of range [0, %d)", pieceIndex, bf.size)
	}
	
	byteIndex := pieceIndex / 8
	bitIndex := uint(pieceIndex % 8)
	
	bf.data[byteIndex] |= (0x80 >> bitIndex)
	return nil
}

// ClearPiece marks a piece as unavailable
func (bf *Bitfield) ClearPiece(pieceIndex int) error {
	if pieceIndex < 0 || pieceIndex >= bf.size {
		return fmt.Errorf("piece index %d out of range [0, %d)", pieceIndex, bf.size)
	}
	
	byteIndex := pieceIndex / 8
	bitIndex := uint(pieceIndex % 8)
	
	bf.data[byteIndex] &^= (0x80 >> bitIndex)
	return nil
}

// HasPiece returns true if the piece is available
func (bf *Bitfield) HasPiece(pieceIndex int) bool {
	if pieceIndex < 0 || pieceIndex >= bf.size {
		return false
	}
	
	byteIndex := pieceIndex / 8
	bitIndex := uint(pieceIndex % 8)
	
	return (bf.data[byteIndex] & (0x80 >> bitIndex)) != 0
}

// IsComplete returns true if all pieces are available
func (bf *Bitfield) IsComplete() bool {
	for i := 0; i < bf.size; i++ {
		if !bf.HasPiece(i) {
			return false
		}
	}
	return true
}

// GetMissingPieces returns a list of missing piece indices
func (bf *Bitfield) GetMissingPieces() []int {
	var missing []int
	for i := 0; i < bf.size; i++ {
		if !bf.HasPiece(i) {
			missing = append(missing, i)
		}
	}
	return missing
}

// GetAvailablePieces returns a list of available piece indices
func (bf *Bitfield) GetAvailablePieces() []int {
	var available []int
	for i := 0; i < bf.size; i++ {
		if bf.HasPiece(i) {
			available = append(available, i)
		}
	}
	return available
}

// GetCompletionPercentage returns the completion percentage (0-100)
func (bf *Bitfield) GetCompletionPercentage() float64 {
	if bf.size == 0 {
		return 100.0
	}
	
	completed := 0
	for i := 0; i < bf.size; i++ {
		if bf.HasPiece(i) {
			completed++
		}
	}
	
	return float64(completed) / float64(bf.size) * 100.0
}

// GetNumPieces returns the total number of pieces
func (bf *Bitfield) GetNumPieces() int {
	return bf.size
}

// GetNumCompletePieces returns the number of complete pieces
func (bf *Bitfield) GetNumCompletePieces() int {
	completed := 0
	for i := 0; i < bf.size; i++ {
		if bf.HasPiece(i) {
			completed++
		}
	}
	return completed
}

// GetNumMissingPieces returns the number of missing pieces
func (bf *Bitfield) GetNumMissingPieces() int {
	return bf.size - bf.GetNumCompletePieces()
}

// ToBytes returns the raw byte representation
func (bf *Bitfield) ToBytes() []byte {
	result := make([]byte, len(bf.data))
	copy(result, bf.data)
	return result
}

// Clone creates a copy of the bitfield
func (bf *Bitfield) Clone() *Bitfield {
	data := make([]byte, len(bf.data))
	copy(data, bf.data)
	
	return &Bitfield{
		data: data,
		size: bf.size,
	}
}

// And performs a bitwise AND with another bitfield
func (bf *Bitfield) And(other *Bitfield) *Bitfield {
	if bf.size != other.size {
		panic("bitfield sizes must match for AND operation")
	}
	
	result := NewBitfield(bf.size)
	minLen := len(bf.data)
	if len(other.data) < minLen {
		minLen = len(other.data)
	}
	
	for i := 0; i < minLen; i++ {
		result.data[i] = bf.data[i] & other.data[i]
	}
	
	return result
}

// Or performs a bitwise OR with another bitfield
func (bf *Bitfield) Or(other *Bitfield) *Bitfield {
	if bf.size != other.size {
		panic("bitfield sizes must match for OR operation")
	}
	
	result := NewBitfield(bf.size)
	maxLen := len(bf.data)
	if len(other.data) > maxLen {
		maxLen = len(other.data)
	}
	
	for i := 0; i < maxLen; i++ {
		var a, b byte
		if i < len(bf.data) {
			a = bf.data[i]
		}
		if i < len(other.data) {
			b = other.data[i]
		}
		if i < len(result.data) {
			result.data[i] = a | b
		}
	}
	
	return result
}

// String returns a string representation of the bitfield
func (bf *Bitfield) String() string {
	if bf.size == 0 {
		return "[]"
	}
	
	result := "["
	for i := 0; i < bf.size; i++ {
		if i > 0 {
			result += " "
		}
		if bf.HasPiece(i) {
			result += "1"
		} else {
			result += "0"
		}
		
		// Limit output for very large bitfields
		if i >= 63 && bf.size > 64 {
			result += fmt.Sprintf(" ... (%d more)", bf.size-64)
			break
		}
	}
	result += "]"
	
	return result
}
