package pieces

import (
	"crypto/sha1"
	"fmt"
	"sync"
)

const (
	// BlockSize is the standard block size for BitTorrent (16KB)
	BlockSize = 16384
)

// PieceManager manages piece downloads and verification
type PieceManager struct {
	mutex         sync.RWMutex
	pieceLength   int
	totalLength   int64
	pieceHashes   [][20]byte
	numPieces     int
	bitfield      *Bitfield
	pendingPieces map[int]*PieceState
	completePieces map[int][]byte
}

// PieceState tracks the state of a piece being downloaded
type PieceState struct {
	Index       int
	Length      int
	Hash        [20]byte
	Downloaded  int
	Blocks      map[int][]byte // block offset -> data
	Requested   map[int]bool   // block offset -> requested
}

// BlockRequest represents a request for a block
type BlockRequest struct {
	PieceIndex int
	Begin      int
	Length     int
}

// NewPieceManager creates a new piece manager
func NewPieceManager(pieceLength int, totalLength int64, pieceHashes [][20]byte) *PieceManager {
	numPieces := len(pieceHashes)
	
	return &PieceManager{
		pieceLength:    pieceLength,
		totalLength:    totalLength,
		pieceHashes:    pieceHashes,
		numPieces:      numPieces,
		bitfield:       NewBitfield(numPieces),
		pendingPieces:  make(map[int]*PieceState),
		completePieces: make(map[int][]byte),
	}
}

// GetBitfield returns a copy of the current bitfield
func (pm *PieceManager) GetBitfield() *Bitfield {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	return pm.bitfield.Clone()
}

// HasPiece returns true if we have the specified piece
func (pm *PieceManager) HasPiece(pieceIndex int) bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	return pm.bitfield.HasPiece(pieceIndex)
}

// GetPieceLength returns the length of a specific piece
func (pm *PieceManager) GetPieceLength(pieceIndex int) int {
	if pieceIndex < 0 || pieceIndex >= pm.numPieces {
		return 0
	}
	
	if pieceIndex == pm.numPieces-1 {
		// Last piece might be shorter
		lastPieceLength := int(pm.totalLength % int64(pm.pieceLength))
		if lastPieceLength == 0 {
			return pm.pieceLength
		}
		return lastPieceLength
	}
	
	return pm.pieceLength
}

// StartPiece begins downloading a piece
func (pm *PieceManager) StartPiece(pieceIndex int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	
	if pieceIndex < 0 || pieceIndex >= pm.numPieces {
		return fmt.Errorf("piece index %d out of range", pieceIndex)
	}
	
	if pm.bitfield.HasPiece(pieceIndex) {
		return fmt.Errorf("piece %d already complete", pieceIndex)
	}
	
	if _, exists := pm.pendingPieces[pieceIndex]; exists {
		return fmt.Errorf("piece %d already in progress", pieceIndex)
	}
	
	pieceLength := pm.GetPieceLength(pieceIndex)
	
	pm.pendingPieces[pieceIndex] = &PieceState{
		Index:      pieceIndex,
		Length:     pieceLength,
		Hash:       pm.pieceHashes[pieceIndex],
		Downloaded: 0,
		Blocks:     make(map[int][]byte),
		Requested:  make(map[int]bool),
	}
	
	return nil
}

// GetNextBlockRequest returns the next block request for a piece
func (pm *PieceManager) GetNextBlockRequest(pieceIndex int) (*BlockRequest, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	
	piece, exists := pm.pendingPieces[pieceIndex]
	if !exists {
		return nil, fmt.Errorf("piece %d not in progress", pieceIndex)
	}
	
	// Find the next unrequested block
	for offset := 0; offset < piece.Length; offset += BlockSize {
		if piece.Requested[offset] {
			continue
		}
		
		if _, hasBlock := piece.Blocks[offset]; hasBlock {
			continue
		}
		
		blockLength := BlockSize
		if offset+blockLength > piece.Length {
			blockLength = piece.Length - offset
		}
		
		piece.Requested[offset] = true
		
		return &BlockRequest{
			PieceIndex: pieceIndex,
			Begin:      offset,
			Length:     blockLength,
		}, nil
	}
	
	return nil, nil // No more blocks to request
}

// AddBlock adds a block to a piece being downloaded
func (pm *PieceManager) AddBlock(pieceIndex, begin int, data []byte) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	
	piece, exists := pm.pendingPieces[pieceIndex]
	if !exists {
		return fmt.Errorf("piece %d not in progress", pieceIndex)
	}
	
	if begin < 0 || begin >= piece.Length {
		return fmt.Errorf("invalid block offset %d for piece %d", begin, pieceIndex)
	}
	
	if begin+len(data) > piece.Length {
		return fmt.Errorf("block extends beyond piece boundary")
	}
	
	// Store the block
	piece.Blocks[begin] = make([]byte, len(data))
	copy(piece.Blocks[begin], data)
	piece.Downloaded += len(data)
	
	// Check if piece is complete
	if pm.isPieceComplete(piece) {
		return pm.completePiece(pieceIndex)
	}
	
	return nil
}

// isPieceComplete checks if all blocks for a piece have been downloaded
func (pm *PieceManager) isPieceComplete(piece *PieceState) bool {
	totalDownloaded := 0
	for offset := 0; offset < piece.Length; offset += BlockSize {
		if block, exists := piece.Blocks[offset]; exists {
			totalDownloaded += len(block)
		} else {
			return false
		}
	}
	return totalDownloaded == piece.Length
}

// completePiece verifies and marks a piece as complete
func (pm *PieceManager) completePiece(pieceIndex int) error {
	piece := pm.pendingPieces[pieceIndex]
	
	// Assemble the complete piece
	pieceData := make([]byte, piece.Length)
	for offset := 0; offset < piece.Length; offset += BlockSize {
		block := piece.Blocks[offset]
		copy(pieceData[offset:], block)
	}
	
	// Verify hash
	hash := sha1.Sum(pieceData)
	if hash != piece.Hash {
		// Hash mismatch, restart the piece
		delete(pm.pendingPieces, pieceIndex)
		return fmt.Errorf("piece %d hash verification failed", pieceIndex)
	}
	
	// Mark piece as complete
	pm.bitfield.SetPiece(pieceIndex)
	pm.completePieces[pieceIndex] = pieceData
	delete(pm.pendingPieces, pieceIndex)
	
	fmt.Printf("Piece %d completed and verified\n", pieceIndex)
	return nil
}

// GetPieceData returns the data for a completed piece
func (pm *PieceManager) GetPieceData(pieceIndex int) ([]byte, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	if !pm.bitfield.HasPiece(pieceIndex) {
		return nil, fmt.Errorf("piece %d not complete", pieceIndex)
	}
	
	if data, exists := pm.completePieces[pieceIndex]; exists {
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	}
	
	return nil, fmt.Errorf("piece %d data not found", pieceIndex)
}

// GetProgress returns download progress information
func (pm *PieceManager) GetProgress() (int, int, float64) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	completed := pm.bitfield.GetNumCompletePieces()
	total := pm.bitfield.GetNumPieces()
	percentage := pm.bitfield.GetCompletionPercentage()
	
	return completed, total, percentage
}

// IsComplete returns true if all pieces are downloaded
func (pm *PieceManager) IsComplete() bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	return pm.bitfield.IsComplete()
}

// GetMissingPieces returns a list of missing piece indices
func (pm *PieceManager) GetMissingPieces() []int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	return pm.bitfield.GetMissingPieces()
}

// CancelPiece cancels downloading of a piece
func (pm *PieceManager) CancelPiece(pieceIndex int) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	
	delete(pm.pendingPieces, pieceIndex)
}

// GetPendingRequests returns the number of pending block requests for a piece
func (pm *PieceManager) GetPendingRequests(pieceIndex int) int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	piece, exists := pm.pendingPieces[pieceIndex]
	if !exists {
		return 0
	}
	
	pending := 0
	for offset := 0; offset < piece.Length; offset += BlockSize {
		if piece.Requested[offset] && piece.Blocks[offset] == nil {
			pending++
		}
	}
	
	return pending
}

// GetPieceProgress returns the download progress of a specific piece
func (pm *PieceManager) GetPieceProgress(pieceIndex int) (int, int) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	if pm.bitfield.HasPiece(pieceIndex) {
		length := pm.GetPieceLength(pieceIndex)
		return length, length
	}
	
	piece, exists := pm.pendingPieces[pieceIndex]
	if !exists {
		return 0, pm.GetPieceLength(pieceIndex)
	}
	
	downloaded := 0
	for _, block := range piece.Blocks {
		downloaded += len(block)
	}
	
	return downloaded, piece.Length
}

// GetAllPieceData returns all completed piece data in order
func (pm *PieceManager) GetAllPieceData() ([]byte, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	if !pm.bitfield.IsComplete() {
		return nil, fmt.Errorf("download not complete")
	}
	
	var result []byte
	for i := 0; i < pm.numPieces; i++ {
		data, exists := pm.completePieces[i]
		if !exists {
			return nil, fmt.Errorf("missing piece %d data", i)
		}
		result = append(result, data...)
	}
	
	return result, nil
}
