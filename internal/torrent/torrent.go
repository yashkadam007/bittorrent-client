package torrent

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yashkadam007/bittorrent-client/internal/bencode"
)

// TorrentFile represents a parsed .torrent file.
// Contains metadata about the torrent including trackers, file info, and calculated info hash.
type TorrentFile struct {
	Announce     string      `json:"announce"`      // Primary tracker URL
	AnnounceList [][]string  `json:"announce_list"` // List of tracker tiers
	Comment      string      `json:"comment"`       // Optional comment
	CreatedBy    string      `json:"created_by"`    // Creator information
	CreationDate int64       `json:"creation_date"` // Unix timestamp
	Info         TorrentInfo `json:"info"`          // File/piece information
	InfoHash     [20]byte    `json:"info_hash"`     // SHA1 hash of info dict
}

// TorrentInfo represents the info dictionary from a torrent file.
// Contains information about files, pieces, and piece hashes.
type TorrentInfo struct {
	Name        string     `json:"name"`         // Name of the torrent
	PieceLength int64      `json:"piece_length"` // Size of each piece in bytes
	Pieces      []byte     `json:"pieces"`       // Concatenated SHA1 hashes (20 bytes each)
	Private     int64      `json:"private"`      // Private torrent flag
	Length      int64      `json:"length"`       // Total size (single file mode)
	Files       []FileInfo `json:"files"`        // File list (multi-file mode)
}

// FileInfo represents a file in multi-file mode.
type FileInfo struct {
	Length int64    `json:"length"` // File size in bytes
	Path   []string `json:"path"`   // File path components
}

// GetPieceHashes extracts individual 20-byte SHA1 hashes from the pieces field.
func (t *TorrentInfo) GetPieceHashes() ([][20]byte, error) {
	if len(t.Pieces)%20 != 0 {
		return nil, fmt.Errorf("invalid pieces length: %d (must be multiple of 20)", len(t.Pieces))
	}

	numPieces := len(t.Pieces) / 20
	hashes := make([][20]byte, numPieces)

	for i := 0; i < numPieces; i++ {
		copy(hashes[i][:], t.Pieces[i*20:(i+1)*20])
	}

	return hashes, nil
}

// GetTotalLength calculates the total size of all files in the torrent.
func (t *TorrentInfo) GetTotalLength() int64 {
	if t.Length > 0 {
		// Single file mode
		return t.Length
	}

	// Multi file mode
	var total int64
	for _, file := range t.Files {
		total += file.Length
	}
	return total
}

// IsMultiFile returns true if this torrent contains multiple files.
func (t *TorrentInfo) IsMultiFile() bool {
	return len(t.Files) > 0
}

// GetNumPieces returns the total number of pieces in the torrent.
func (t *TorrentInfo) GetNumPieces() int {
	return len(t.Pieces) / 20
}

// GetLastPieceLength calculates the size of the final piece (may be shorter than piece_length).
func (t *TorrentInfo) GetLastPieceLength() int64 {
	totalLength := t.GetTotalLength()
	numPieces := t.GetNumPieces()

	if numPieces == 0 {
		return 0
	}

	lastPieceLength := totalLength % t.PieceLength
	if lastPieceLength == 0 {
		return t.PieceLength
	}
	return lastPieceLength
}

// ParseTorrentFile reads and parses a .torrent file from disk.
// Returns a TorrentFile struct with all metadata and calculated info hash.
func ParseTorrentFile(filePath string) (*TorrentFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w", err)
	}
	defer file.Close()

	decoder := bencode.NewDecoder(file)
	data, err := decoder.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to decode torrent file: %w", err)
	}

	dict, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent file root is not a dictionary")
	}

	torrent := &TorrentFile{}

	// Parse announce
	if announce, ok := dict["announce"].([]byte); ok {
		torrent.Announce = string(announce)
	} else {
		return nil, fmt.Errorf("missing or invalid announce field")
	}

	// Parse announce-list (optional)
	if announceList, ok := dict["announce-list"].([]interface{}); ok {
		for _, tierInterface := range announceList {
			if tier, ok := tierInterface.([]interface{}); ok {
				var tierStrings []string
				for _, urlInterface := range tier {
					if urlBytes, ok := urlInterface.([]byte); ok {
						tierStrings = append(tierStrings, string(urlBytes))
					}
				}
				if len(tierStrings) > 0 {
					torrent.AnnounceList = append(torrent.AnnounceList, tierStrings)
				}
			}
		}
	}

	// Parse optional metadata fields
	if comment, ok := dict["comment"].([]byte); ok {
		torrent.Comment = string(comment)
	}
	if createdBy, ok := dict["created by"].([]byte); ok {
		torrent.CreatedBy = string(createdBy)
	}
	if creationDate, ok := dict["creation date"].(int64); ok {
		torrent.CreationDate = creationDate
	}

	// Parse info dictionary
	infoInterface, ok := dict["info"]
	if !ok {
		return nil, fmt.Errorf("missing info dictionary")
	}

	infoDict, ok := infoInterface.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("info is not a dictionary")
	}

	err = torrent.parseInfo(infoDict)
	if err != nil {
		return nil, fmt.Errorf("failed to parse info dictionary: %w", err)
	}

	// Calculate info hash
	err = torrent.calculateInfoHash(infoDict)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate info hash: %w", err)
	}

	return torrent, nil
}

// parseInfo extracts file and piece information from the info dictionary.
func (t *TorrentFile) parseInfo(infoDict map[string]interface{}) error {
	// Parse name
	nameBytes, ok := infoDict["name"].([]byte)
	if !ok {
		return fmt.Errorf("missing or invalid name field")
	}
	t.Info.Name = string(nameBytes)

	// Parse piece length
	pieceLength, ok := infoDict["piece length"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid piece length field")
	}
	t.Info.PieceLength = pieceLength

	// Parse pieces
	pieces, ok := infoDict["pieces"].([]byte)
	if !ok {
		return fmt.Errorf("missing or invalid pieces field")
	}
	if len(pieces)%20 != 0 {
		return fmt.Errorf("invalid pieces length: %d (must be multiple of 20)", len(pieces))
	}
	t.Info.Pieces = pieces

	// Parse private (optional)
	if private, ok := infoDict["private"].(int64); ok {
		t.Info.Private = private
	}

	// Determine torrent mode and parse file information
	if length, ok := infoDict["length"].(int64); ok {
		// Single file torrent
		t.Info.Length = length
	} else if filesInterface, ok := infoDict["files"].([]interface{}); ok {
		// Multi file mode
		for _, fileInterface := range filesInterface {
			fileDict, ok := fileInterface.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid file dictionary")
			}

			fileInfo := FileInfo{}

			// Parse file length
			if length, ok := fileDict["length"].(int64); ok {
				fileInfo.Length = length
			} else {
				return fmt.Errorf("missing or invalid file length")
			}

			// md5sum is optional and rarely used, so we skip it for simplicity

			// Parse path
			pathInterface, ok := fileDict["path"].([]interface{})
			if !ok {
				return fmt.Errorf("missing or invalid file path")
			}

			for _, pathComponent := range pathInterface {
				if pathBytes, ok := pathComponent.([]byte); ok {
					fileInfo.Path = append(fileInfo.Path, string(pathBytes))
				} else {
					return fmt.Errorf("invalid path component")
				}
			}

			if len(fileInfo.Path) == 0 {
				return fmt.Errorf("empty file path")
			}

			t.Info.Files = append(t.Info.Files, fileInfo)
		}
	} else {
		return fmt.Errorf("missing length (single file) or files (multi file) field")
	}

	return nil
}

// calculateInfoHash computes the SHA1 hash of the info dictionary.
// This hash is used to identify the torrent in the protocol.
func (t *TorrentFile) calculateInfoHash(infoDict map[string]interface{}) error {
	// Re-encode the info dictionary to calculate hash
	var buf strings.Builder
	encoder := bencode.NewEncoder(&buf)
	err := encoder.Encode(infoDict)
	if err != nil {
		return fmt.Errorf("failed to encode info dictionary: %w", err)
	}

	hash := sha1.Sum([]byte(buf.String()))
	t.InfoHash = hash
	return nil
}

// GetOutputPath determines where files should be saved based on torrent type.
func (t *TorrentFile) GetOutputPath(baseDir string) string {
	if baseDir == "" {
		baseDir = "."
	}

	if t.Info.IsMultiFile() {
		return filepath.Join(baseDir, t.Info.Name)
	}
	return filepath.Join(baseDir, t.Info.Name)
}

// GetAllTrackers combines primary tracker and announce-list into a single slice.
func (t *TorrentFile) GetAllTrackers() []string {
	trackers := []string{t.Announce}

	for _, tier := range t.AnnounceList {
		for _, tracker := range tier {
			// Avoid duplicates
			found := false
			for _, existing := range trackers {
				if existing == tracker {
					found = true
					break
				}
			}
			if !found {
				trackers = append(trackers, tracker)
			}
		}
	}

	return trackers
}

// String provides a human-readable summary of the torrent information.
func (t *TorrentFile) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Name: %s\n", t.Info.Name))
	sb.WriteString(fmt.Sprintf("Announce: %s\n", t.Announce))
	sb.WriteString(fmt.Sprintf("Info Hash: %x\n", t.InfoHash))
	sb.WriteString(fmt.Sprintf("Piece Length: %d bytes\n", t.Info.PieceLength))
	sb.WriteString(fmt.Sprintf("Number of Pieces: %d\n", t.Info.GetNumPieces()))
	sb.WriteString(fmt.Sprintf("Total Size: %d bytes\n", t.Info.GetTotalLength()))

	if t.Info.IsMultiFile() {
		sb.WriteString(fmt.Sprintf("Files: %d\n", len(t.Info.Files)))
		for i, file := range t.Info.Files {
			sb.WriteString(fmt.Sprintf("  %d. %s (%d bytes)\n", i+1,
				filepath.Join(file.Path...), file.Length))
		}
	} else {
		sb.WriteString("Single file torrent\n")
	}

	if t.Comment != "" {
		sb.WriteString(fmt.Sprintf("Comment: %s\n", t.Comment))
	}
	if t.CreatedBy != "" {
		sb.WriteString(fmt.Sprintf("Created by: %s\n", t.CreatedBy))
	}

	return sb.String()
}
