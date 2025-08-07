package torrent

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yashkadam007/bittorrent-client/internal/bencode"
)

// TorrentFile represents a parsed .torrent file
type TorrentFile struct {
	Announce     string      `json:"announce"`
	AnnounceList [][]string  `json:"announce_list,omitempty"`
	Comment      string      `json:"comment,omitempty"`
	CreatedBy    string      `json:"created_by,omitempty"`
	CreationDate int64       `json:"creation_date,omitempty"`
	Encoding     string      `json:"encoding,omitempty"`
	Info         TorrentInfo `json:"info"`
	InfoHash     [20]byte    `json:"info_hash"`
}

// TorrentInfo represents the info dictionary from a torrent file
type TorrentInfo struct {
	Name        string     `json:"name"`
	PieceLength int64      `json:"piece_length"`
	Pieces      []byte     `json:"pieces"`
	Private     int64      `json:"private,omitempty"`
	Length      int64      `json:"length,omitempty"` // Single file mode
	MD5Sum      string     `json:"md5sum,omitempty"` // Single file mode
	Files       []FileInfo `json:"files,omitempty"`  // Multi file mode
}

// FileInfo represents a file in multi-file mode
type FileInfo struct {
	Length int64    `json:"length"`
	MD5Sum string   `json:"md5sum,omitempty"`
	Path   []string `json:"path"`
}

// GetPieceHashes returns the individual piece hashes
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

// GetTotalLength returns the total length of all files
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

// IsMultiFile returns true if this is a multi-file torrent
func (t *TorrentInfo) IsMultiFile() bool {
	return len(t.Files) > 0
}

// GetNumPieces returns the number of pieces
func (t *TorrentInfo) GetNumPieces() int {
	return len(t.Pieces) / 20
}

// GetLastPieceLength returns the length of the last piece
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

// ParseTorrentFile parses a .torrent file
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

	// Parse optional fields
	if comment, ok := dict["comment"].([]byte); ok {
		torrent.Comment = string(comment)
	}
	if createdBy, ok := dict["created by"].([]byte); ok {
		torrent.CreatedBy = string(createdBy)
	}
	if creationDate, ok := dict["creation date"].(int64); ok {
		torrent.CreationDate = creationDate
	}
	if encoding, ok := dict["encoding"].([]byte); ok {
		torrent.Encoding = string(encoding)
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

	// Check if single file or multi file mode
	if length, ok := infoDict["length"].(int64); ok {
		// Single file mode
		t.Info.Length = length
		if md5sum, ok := infoDict["md5sum"].([]byte); ok {
			t.Info.MD5Sum = string(md5sum)
		}
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

			// Parse md5sum (optional)
			if md5sum, ok := fileDict["md5sum"].([]byte); ok {
				fileInfo.MD5Sum = string(md5sum)
			}

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

// GetOutputPath returns the path where files should be saved
func (t *TorrentFile) GetOutputPath(baseDir string) string {
	if baseDir == "" {
		baseDir = "."
	}

	if t.Info.IsMultiFile() {
		return filepath.Join(baseDir, t.Info.Name)
	}
	return filepath.Join(baseDir, t.Info.Name)
}

// GetAllTrackers returns all trackers from announce and announce-list
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

// String returns a string representation of the torrent
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
