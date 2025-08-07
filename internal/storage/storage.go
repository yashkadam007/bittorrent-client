package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/yashkadam007/bittorrent-client/internal/pieces"
	"github.com/yashkadam007/bittorrent-client/internal/torrent"
)

// FileStorage handles reading and writing files for the torrent
type FileStorage struct {
	torrent     *torrent.TorrentFile
	baseDir     string
	files       []*os.File
	fileInfos   []FileInfo
	totalLength int64
	mutex       sync.RWMutex
}

// FileInfo contains information about a file in the torrent
type FileInfo struct {
	Path   string
	Length int64
	Offset int64 // Offset within the concatenated file data
}

// NewFileStorage creates a new file storage instance
func NewFileStorage(t *torrent.TorrentFile, baseDir string) (*FileStorage, error) {
	if baseDir == "" {
		baseDir = "."
	}

	fs := &FileStorage{
		torrent:     t,
		baseDir:     baseDir,
		totalLength: t.Info.GetTotalLength(),
	}

	err := fs.setupFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to setup files: %w", err)
	}

	return fs, nil
}

// setupFiles creates the file structure and opens files
func (fs *FileStorage) setupFiles() error {
	if fs.torrent.Info.IsMultiFile() {
		// Multi-file torrent
		baseDir := filepath.Join(fs.baseDir, fs.torrent.Info.Name)
		err := os.MkdirAll(baseDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create base directory: %w", err)
		}

		var offset int64
		for _, fileInfo := range fs.torrent.Info.Files {
			// Create full file path
			fullPath := filepath.Join(baseDir, filepath.Join(fileInfo.Path...))
			
			// Create directory if needed
			dir := filepath.Dir(fullPath)
			err := os.MkdirAll(dir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}

			fs.fileInfos = append(fs.fileInfos, FileInfo{
				Path:   fullPath,
				Length: fileInfo.Length,
				Offset: offset,
			})

			offset += fileInfo.Length
		}
	} else {
		// Single file torrent
		fullPath := filepath.Join(fs.baseDir, fs.torrent.Info.Name)
		
		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if dir != "." {
			err := os.MkdirAll(dir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		fs.fileInfos = append(fs.fileInfos, FileInfo{
			Path:   fullPath,
			Length: fs.torrent.Info.Length,
			Offset: 0,
		})
	}

	// Open all files
	fs.files = make([]*os.File, len(fs.fileInfos))
	for i, fileInfo := range fs.fileInfos {
		file, err := os.OpenFile(fileInfo.Path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			// Close already opened files
			for j := 0; j < i; j++ {
				fs.files[j].Close()
			}
			return fmt.Errorf("failed to open file %s: %w", fileInfo.Path, err)
		}

		// Ensure file has correct size
		err = file.Truncate(fileInfo.Length)
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to set file size for %s: %w", fileInfo.Path, err)
		}

		fs.files[i] = file
	}

	return nil
}

// ReadPiece reads a complete piece from storage
func (fs *FileStorage) ReadPiece(pieceIndex int) ([]byte, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	if pieceIndex < 0 || pieceIndex >= fs.torrent.Info.GetNumPieces() {
		return nil, fmt.Errorf("piece index %d out of range", pieceIndex)
	}

	pieceLength := fs.getPieceLength(pieceIndex)
	offset := int64(pieceIndex) * int64(fs.torrent.Info.PieceLength)

	data := make([]byte, pieceLength)
	_, err := fs.readAt(data, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to read piece %d: %w", pieceIndex, err)
	}

	return data, nil
}

// WritePiece writes a complete piece to storage
func (fs *FileStorage) WritePiece(pieceIndex int, data []byte) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if pieceIndex < 0 || pieceIndex >= fs.torrent.Info.GetNumPieces() {
		return fmt.Errorf("piece index %d out of range", pieceIndex)
	}

	expectedLength := fs.getPieceLength(pieceIndex)
	if len(data) != expectedLength {
		return fmt.Errorf("piece %d has incorrect length: got %d, expected %d", 
			pieceIndex, len(data), expectedLength)
	}

	offset := int64(pieceIndex) * int64(fs.torrent.Info.PieceLength)
	_, err := fs.writeAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write piece %d: %w", pieceIndex, err)
	}

	return nil
}

// ReadBlock reads a block from storage
func (fs *FileStorage) ReadBlock(pieceIndex, begin, length int) ([]byte, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	if pieceIndex < 0 || pieceIndex >= fs.torrent.Info.GetNumPieces() {
		return nil, fmt.Errorf("piece index %d out of range", pieceIndex)
	}

	pieceLength := fs.getPieceLength(pieceIndex)
	if begin < 0 || begin >= pieceLength {
		return nil, fmt.Errorf("block begin %d out of range for piece %d", begin, pieceIndex)
	}

	if begin+length > pieceLength {
		return nil, fmt.Errorf("block extends beyond piece boundary")
	}

	offset := int64(pieceIndex)*int64(fs.torrent.Info.PieceLength) + int64(begin)
	data := make([]byte, length)
	
	_, err := fs.readAt(data, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to read block: %w", err)
	}

	return data, nil
}

// WriteBlock writes a block to storage
func (fs *FileStorage) WriteBlock(pieceIndex, begin int, data []byte) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if pieceIndex < 0 || pieceIndex >= fs.torrent.Info.GetNumPieces() {
		return fmt.Errorf("piece index %d out of range", pieceIndex)
	}

	pieceLength := fs.getPieceLength(pieceIndex)
	if begin < 0 || begin >= pieceLength {
		return fmt.Errorf("block begin %d out of range for piece %d", begin, pieceIndex)
	}

	if begin+len(data) > pieceLength {
		return fmt.Errorf("block extends beyond piece boundary")
	}

	offset := int64(pieceIndex)*int64(fs.torrent.Info.PieceLength) + int64(begin)
	_, err := fs.writeAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write block: %w", err)
	}

	return nil
}

// readAt reads data from the specified offset across multiple files
func (fs *FileStorage) readAt(data []byte, offset int64) (int, error) {
	if offset < 0 || offset >= fs.totalLength {
		return 0, fmt.Errorf("offset %d out of range", offset)
	}

	totalRead := 0
	remaining := len(data)

	for i, fileInfo := range fs.fileInfos {
		if offset >= fileInfo.Offset+fileInfo.Length {
			continue
		}

		if offset < fileInfo.Offset {
			break
		}

		// Calculate read parameters for this file
		fileOffset := offset - fileInfo.Offset
		maxRead := int(fileInfo.Length - fileOffset)
		if maxRead > remaining {
			maxRead = remaining
		}

		// Read from file
		n, err := fs.files[i].ReadAt(data[totalRead:totalRead+maxRead], fileOffset)
		totalRead += n

		if err != nil && err != io.EOF {
			return totalRead, err
		}

		remaining -= n
		offset += int64(n)

		if remaining == 0 {
			break
		}
	}

	return totalRead, nil
}

// writeAt writes data to the specified offset across multiple files
func (fs *FileStorage) writeAt(data []byte, offset int64) (int, error) {
	if offset < 0 || offset >= fs.totalLength {
		return 0, fmt.Errorf("offset %d out of range", offset)
	}

	totalWritten := 0
	remaining := len(data)

	for i, fileInfo := range fs.fileInfos {
		if offset >= fileInfo.Offset+fileInfo.Length {
			continue
		}

		if offset < fileInfo.Offset {
			break
		}

		// Calculate write parameters for this file
		fileOffset := offset - fileInfo.Offset
		maxWrite := int(fileInfo.Length - fileOffset)
		if maxWrite > remaining {
			maxWrite = remaining
		}

		// Write to file
		n, err := fs.files[i].WriteAt(data[totalWritten:totalWritten+maxWrite], fileOffset)
		totalWritten += n

		if err != nil {
			return totalWritten, err
		}

		remaining -= n
		offset += int64(n)

		if remaining == 0 {
			break
		}
	}

	return totalWritten, nil
}

// getPieceLength returns the length of a specific piece
func (fs *FileStorage) getPieceLength(pieceIndex int) int {
	if pieceIndex == fs.torrent.Info.GetNumPieces()-1 {
		// Last piece might be shorter
		lastPieceLength := int(fs.totalLength % int64(fs.torrent.Info.PieceLength))
		if lastPieceLength == 0 {
			return int(fs.torrent.Info.PieceLength)
		}
		return lastPieceLength
	}
	return int(fs.torrent.Info.PieceLength)
}

// Sync flushes all file buffers to disk
func (fs *FileStorage) Sync() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	for i, file := range fs.files {
		if file != nil {
			err := file.Sync()
			if err != nil {
				return fmt.Errorf("failed to sync file %s: %w", fs.fileInfos[i].Path, err)
			}
		}
	}

	return nil
}

// Close closes all open files
func (fs *FileStorage) Close() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	var lastError error
	for i, file := range fs.files {
		if file != nil {
			err := file.Close()
			if err != nil {
				lastError = fmt.Errorf("failed to close file %s: %w", fs.fileInfos[i].Path, err)
			}
			fs.files[i] = nil
		}
	}

	return lastError
}

// GetCompletionBitfield scans existing files to determine which pieces are complete
func (fs *FileStorage) GetCompletionBitfield() (*pieces.Bitfield, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	numPieces := fs.torrent.Info.GetNumPieces()
	bitfield := pieces.NewBitfield(numPieces)
	
	pieceHashes, err := fs.torrent.Info.GetPieceHashes()
	if err != nil {
		return nil, fmt.Errorf("failed to get piece hashes: %w", err)
	}

	// Check each piece
	for i := 0; i < numPieces; i++ {
		data, err := fs.ReadPiece(i)
		if err != nil {
			continue // Piece not available
		}

		// Verify hash
		if pieces.VerifyPieceHash(data, pieceHashes[i]) {
			bitfield.SetPiece(i)
		}
	}

	return bitfield, nil
}

// GetFileInfos returns information about all files
func (fs *FileStorage) GetFileInfos() []FileInfo {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	result := make([]FileInfo, len(fs.fileInfos))
	copy(result, fs.fileInfos)
	return result
}

// GetTotalLength returns the total length of all files
func (fs *FileStorage) GetTotalLength() int64 {
	return fs.totalLength
}

// GetProgress returns the current download progress by checking file sizes
func (fs *FileStorage) GetProgress() (int64, int64, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	var downloaded int64
	for i, fileInfo := range fs.fileInfos {
		if fs.files[i] != nil {
			stat, err := fs.files[i].Stat()
			if err != nil {
				continue
			}
			
			fileSize := stat.Size()
			if fileSize > fileInfo.Length {
				fileSize = fileInfo.Length
			}
			downloaded += fileSize
		}
	}

	return downloaded, fs.totalLength, nil
}
