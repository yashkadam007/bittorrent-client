package download

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/yashkadam007/bittorrent-client/internal/peer"
	"github.com/yashkadam007/bittorrent-client/internal/pieces"
	"github.com/yashkadam007/bittorrent-client/internal/tracker"
)

// PieceStrategy defines how to select the next piece to download.
// Different strategies can optimize for different goals (speed, rarity, etc.).
type PieceStrategy interface {
	SelectPiece(availablePieces []int, peerBitfield *pieces.Bitfield) (int, error)
}

// RandomStrategy selects pieces randomly from available pieces.
// Simple but not optimal for download efficiency.
type RandomStrategy struct{}

func (rs *RandomStrategy) SelectPiece(availablePieces []int, peerBitfield *pieces.Bitfield) (int, error) {
	if len(availablePieces) == 0 {
		return -1, fmt.Errorf("no available pieces")
	}

	// Find pieces that both we need and the peer has
	var validPieces []int
	for _, pieceIndex := range availablePieces {
		if peerBitfield.HasPiece(pieceIndex) {
			validPieces = append(validPieces, pieceIndex)
		}
	}

	if len(validPieces) == 0 {
		return -1, fmt.Errorf("peer has no pieces we need")
	}

	return validPieces[rand.Intn(len(validPieces))], nil
}

// RarestFirstStrategy prioritizes pieces that are rarest among all peers.
// This helps improve overall swarm health by distributing rare pieces.
type RarestFirstStrategy struct {
	pieceCounts map[int]int  // Track how many peers have each piece
	mutex       sync.RWMutex // Protect concurrent access to piece counts
}

// NewRarestFirstStrategy creates a new rarest-first piece selection strategy.
func NewRarestFirstStrategy() *RarestFirstStrategy {
	return &RarestFirstStrategy{
		pieceCounts: make(map[int]int),
	}
}

// UpdatePeerBitfield updates piece rarity counts when a peer connects.
func (rfs *RarestFirstStrategy) UpdatePeerBitfield(peerBitfield *pieces.Bitfield) {
	rfs.mutex.Lock()
	defer rfs.mutex.Unlock()

	for i := 0; i < peerBitfield.GetNumPieces(); i++ {
		if peerBitfield.HasPiece(i) {
			rfs.pieceCounts[i]++
		}
	}
}

func (rfs *RarestFirstStrategy) SelectPiece(availablePieces []int, peerBitfield *pieces.Bitfield) (int, error) {
	if len(availablePieces) == 0 {
		return -1, fmt.Errorf("no available pieces")
	}

	rfs.mutex.RLock()
	defer rfs.mutex.RUnlock()

	// Filter pieces that the peer has and sort by rarity
	type PieceRarity struct {
		Index int
		Count int
	}

	var validPieces []PieceRarity
	for _, pieceIndex := range availablePieces {
		if peerBitfield.HasPiece(pieceIndex) {
			count := rfs.pieceCounts[pieceIndex]
			validPieces = append(validPieces, PieceRarity{Index: pieceIndex, Count: count})
		}
	}

	if len(validPieces) == 0 {
		return -1, fmt.Errorf("peer has no pieces we need")
	}

	// Sort by count (rarest first)
	sort.Slice(validPieces, func(i, j int) bool {
		return validPieces[i].Count < validPieces[j].Count
	})

	return validPieces[0].Index, nil
}

// DownloadManager coordinates the entire download process.
// It manages peer connections, piece requests, and download progress.
type DownloadManager struct {
	pieceManager *pieces.PieceManager       // Manages piece state and verification
	strategy     PieceStrategy              // Piece selection strategy
	peers        map[string]*PeerConnection // Active peer connections
	maxPeers     int                        // Maximum concurrent peer connections
	mutex        sync.RWMutex               // Protects shared state
	active       bool                       // Is the download manager running?
	stats        *DownloadStats             // Download statistics
	quiet        bool                       // Suppress stdout output (for TUI mode)
}

// PeerConnection wraps a peer connection with download-specific state.
type PeerConnection struct {
	conn            *peer.Connection                // The actual peer connection
	addr            string                          // Peer address for identification
	pendingRequests map[string]*pieces.BlockRequest // Outstanding block requests
	maxRequests     int                             // Max concurrent requests to this peer
	downloadedBytes int64                           // Bytes downloaded from this peer
	lastActivity    time.Time                       // Last time we heard from this peer
	mutex           sync.Mutex                      // Protects peer-specific state
}

// DownloadStats tracks download progress and performance metrics.
type DownloadStats struct {
	DownloadedBytes int64     // Total bytes downloaded
	DownloadSpeed   float64   // Current download speed (bytes/second)
	StartTime       time.Time // When the download started
	PeersConnected  int       // Number of active peer connections
}

// NewDownloadManager creates a new download manager with the given piece manager and strategy.
func NewDownloadManager(pieceManager *pieces.PieceManager, strategy PieceStrategy) *DownloadManager {
	return NewDownloadManagerWithOptions(pieceManager, strategy, false)
}

// NewDownloadManagerWithOptions creates a new download manager with additional options.
func NewDownloadManagerWithOptions(pieceManager *pieces.PieceManager, strategy PieceStrategy, quiet bool) *DownloadManager {
	return &DownloadManager{
		pieceManager: pieceManager,
		strategy:     strategy,
		peers:        make(map[string]*PeerConnection),
		maxPeers:     50,
		quiet:        quiet,
		stats: &DownloadStats{
			StartTime: time.Now(),
		},
	}
}

// AddPeers adds peers from tracker response
func (dm *DownloadManager) AddPeers(peers []tracker.PeerInfo, infoHash, peerID [20]byte) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	for _, peerInfo := range peers {
		if !tracker.IsValidPeer(peerInfo) {
			continue
		}

		addr := fmt.Sprintf("%s:%d", peerInfo.IP, peerInfo.Port)

		// Skip if already connected
		if _, exists := dm.peers[addr]; exists {
			continue
		}

		// Skip if we have too many peers
		if len(dm.peers) >= dm.maxPeers {
			break
		}

		// Connect to peer
		go dm.connectToPeer(addr, infoHash, peerID)
	}
}

func (dm *DownloadManager) connectToPeer(addr string, infoHash, peerID [20]byte) {
	conn, err := peer.Connect(addr, infoHash, peerID)
	if err != nil {
		if !dm.quiet {
			fmt.Printf("Failed to connect to peer %s: %v\n", addr, err)
		}
		return
	}

	peerConn := &PeerConnection{
		conn:            conn,
		addr:            addr,
		pendingRequests: make(map[string]*pieces.BlockRequest),
		maxRequests:     10,
		lastActivity:    time.Now(),
	}

	dm.mutex.Lock()
	dm.peers[addr] = peerConn
	dm.stats.PeersConnected++
	dm.mutex.Unlock()

	if !dm.quiet {
		fmt.Printf("Connected to peer %s\n", addr)
	}

	// Start message handling
	go dm.handlePeer(peerConn)
}

func (dm *DownloadManager) handlePeer(peerConn *PeerConnection) {
	defer func() {
		dm.removePeer(peerConn.addr)
		peerConn.conn.Close()
	}()

	// Send interested message
	err := peerConn.conn.SendInterested()
	if err != nil {
		if !dm.quiet {
			fmt.Printf("Failed to send interested to %s: %v\n", peerConn.addr, err)
		}
		return
	}

	// Start keep-alive routine
	go dm.keepAlive(peerConn)

	// Start request routine
	go dm.requestBlocks(peerConn)

	// Message loop
	for dm.active {
		msg, err := peerConn.conn.ReceiveMessage()
		if err != nil {
			if !dm.quiet {
				fmt.Printf("Error receiving message from %s: %v\n", peerConn.addr, err)
			}
			return
		}

		peerConn.lastActivity = time.Now()

		err = dm.handleMessage(peerConn, msg)
		if err != nil {
			if !dm.quiet {
				fmt.Printf("Error handling message from %s: %v\n", peerConn.addr, err)
			}
			return
		}
	}
}

func (dm *DownloadManager) handleMessage(peerConn *PeerConnection, msg *peer.Message) error {
	switch msg.Type {
	case peer.MsgUnchoke:
		// Start requesting pieces
		go dm.requestBlocks(peerConn)

	case peer.MsgPiece:
		if len(msg.Payload) < 8 {
			return fmt.Errorf("invalid piece message")
		}

		pieceIndex := int(uint32(msg.Payload[0])<<24 | uint32(msg.Payload[1])<<16 | uint32(msg.Payload[2])<<8 | uint32(msg.Payload[3]))
		begin := int(uint32(msg.Payload[4])<<24 | uint32(msg.Payload[5])<<16 | uint32(msg.Payload[6])<<8 | uint32(msg.Payload[7]))
		data := msg.Payload[8:]

		// Remove from pending requests
		peerConn.mutex.Lock()
		key := fmt.Sprintf("%d:%d", pieceIndex, begin)
		delete(peerConn.pendingRequests, key)
		peerConn.downloadedBytes += int64(len(data))
		peerConn.mutex.Unlock()

		// Add block to piece manager
		err := dm.pieceManager.AddBlock(pieceIndex, begin, data)
		if err != nil {
			if !dm.quiet {
				fmt.Printf("Failed to add block: %v\n", err)
			}
		}

		// Update stats
		dm.updateDownloadStats(int64(len(data)))

		// Request more blocks
		go dm.requestBlocks(peerConn)
	}

	// Handle message in peer connection
	return peerConn.conn.HandleMessage(msg)
}

func (dm *DownloadManager) requestBlocks(peerConn *PeerConnection) {
	if peerConn.conn.IsChoked() {
		return
	}

	peerConn.mutex.Lock()
	pendingCount := len(peerConn.pendingRequests)
	peerConn.mutex.Unlock()

	if pendingCount >= peerConn.maxRequests {
		return
	}

	// Get missing pieces
	missingPieces := dm.pieceManager.GetMissingPieces()
	if len(missingPieces) == 0 {
		return
	}

	// Get peer bitfield
	peerBitfield := pieces.NewBitfieldFromBytes(
		peerConn.conn.GetBitfield(),
		dm.pieceManager.GetBitfield().GetNumPieces(),
	)

	// Select piece to download
	pieceIndex, err := dm.strategy.SelectPiece(missingPieces, peerBitfield)
	if err != nil {
		return
	}

	// Start piece if not already started
	err = dm.pieceManager.StartPiece(pieceIndex)
	if err != nil && err.Error() != fmt.Sprintf("piece %d already in progress", pieceIndex) {
		return
	}

	// Request blocks for this piece
	for pendingCount < peerConn.maxRequests {
		blockReq, err := dm.pieceManager.GetNextBlockRequest(pieceIndex)
		if err != nil || blockReq == nil {
			break
		}

		// Send request
		err = peerConn.conn.SendRequest(blockReq.PieceIndex, blockReq.Begin, blockReq.Length)
		if err != nil {
			if !dm.quiet {
				fmt.Printf("Failed to send request to %s: %v\n", peerConn.addr, err)
			}
			break
		}

		// Track pending request
		peerConn.mutex.Lock()
		key := fmt.Sprintf("%d:%d", blockReq.PieceIndex, blockReq.Begin)
		peerConn.pendingRequests[key] = blockReq
		pendingCount++
		peerConn.mutex.Unlock()
	}
}

func (dm *DownloadManager) keepAlive(peerConn *PeerConnection) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for dm.active {
		<-ticker.C
		if time.Since(peerConn.lastActivity) > 3*time.Minute {
			// Peer is inactive, disconnect
			if !dm.quiet {
				fmt.Printf("Peer %s inactive, disconnecting\n", peerConn.addr)
			}
			return
		}

		err := peerConn.conn.SendKeepAlive()
		if err != nil {
			if !dm.quiet {
				fmt.Printf("Failed to send keep-alive to %s: %v\n", peerConn.addr, err)
			}
			return
		}
	}
}

func (dm *DownloadManager) removePeer(addr string) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if _, exists := dm.peers[addr]; exists {
		delete(dm.peers, addr)
		dm.stats.PeersConnected--
		if !dm.quiet {
			fmt.Printf("Disconnected from peer %s\n", addr)
		}
	}
}

func (dm *DownloadManager) updateDownloadStats(bytes int64) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.stats.DownloadedBytes += bytes

	// Calculate download speed (simple moving average)
	elapsed := time.Since(dm.stats.StartTime).Seconds()
	if elapsed > 0 {
		dm.stats.DownloadSpeed = float64(dm.stats.DownloadedBytes) / elapsed
	}
}

// Start begins the download process
func (dm *DownloadManager) Start() {
	dm.mutex.Lock()
	dm.active = true
	dm.mutex.Unlock()

	fmt.Println("Download started")
}

// Stop stops the download process
func (dm *DownloadManager) Stop() {
	dm.mutex.Lock()
	dm.active = false

	// Close all peer connections
	for _, peerConn := range dm.peers {
		peerConn.conn.Close()
	}
	dm.peers = make(map[string]*PeerConnection)
	dm.mutex.Unlock()

	fmt.Println("Download stopped")
}

// IsActive returns true if the download is active
func (dm *DownloadManager) IsActive() bool {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	return dm.active
}

// GetStats returns current download statistics
func (dm *DownloadManager) GetStats() DownloadStats {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	// Return a copy of the stats with current peer count
	stats := *dm.stats
	stats.PeersConnected = len(dm.peers)

	return stats
}

// GetProgress returns download progress
func (dm *DownloadManager) GetProgress() (int, int, float64) {
	return dm.pieceManager.GetProgress()
}

// IsComplete returns true if download is complete
func (dm *DownloadManager) IsComplete() bool {
	return dm.pieceManager.IsComplete()
}
