package tracker

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yashkadam007/bittorrent-client/internal/bencode"
	"github.com/yashkadam007/bittorrent-client/internal/torrent"
)

// TrackerResponse represents a response from a BitTorrent tracker.
type TrackerResponse struct {
	FailureReason  string     `json:"failure_reason"`  // Error message if request failed
	WarningMessage string     `json:"warning_message"` // Optional warning from tracker
	Interval       int64      `json:"interval"`        // Seconds between announcements
	MinInterval    int64      `json:"min_interval"`    // Minimum announce interval
	TrackerID      string     `json:"tracker_id"`      // Tracker session ID
	Complete       int64      `json:"complete"`        // Number of seeders
	Incomplete     int64      `json:"incomplete"`      // Number of leechers
	Peers          []PeerInfo `json:"peers"`           // List of available peers
}

// PeerInfo represents information about a single peer from the tracker.
type PeerInfo struct {
	ID   []byte `json:"id"`   // Peer ID (optional, for dictionary format)
	IP   string `json:"ip"`   // Peer's IP address
	Port int    `json:"port"` // Peer's listening port
}

// TrackerRequest represents parameters for a tracker announce request.
type TrackerRequest struct {
	InfoHash   [20]byte // Torrent identifier
	PeerID     [20]byte // Our client identifier
	Port       int      // Our listening port
	Downloaded int64    // Bytes downloaded so far
	Left       int64    // Bytes remaining to download
	Event      string   // "started", "completed", "stopped", or ""
	NumWant    int      // Number of peers we want
	Key        uint32   // Random key for tracker session
}

// TrackerClient handles communication with BitTorrent trackers.
// Supports both HTTP/HTTPS and UDP tracker protocols.
type TrackerClient struct {
	httpClient *http.Client // HTTP client for tracker requests
	peerID     [20]byte     // Our unique peer identifier
	key        uint32       // Random session key
}

// NewTrackerClient creates a new tracker client with a random peer ID.
func NewTrackerClient() *TrackerClient {
	var peerID [20]byte
	copy(peerID[:], "-GO0001-")
	rand.Read(peerID[8:])

	var key uint32
	binary.Read(rand.Reader, binary.BigEndian, &key)

	return &TrackerClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		peerID: peerID,
		key:    key,
	}
}

// GetPeers requests a list of peers from the tracker.
// Tries all available trackers until one succeeds.
func (tc *TrackerClient) GetPeers(t *torrent.TorrentFile, port int, event string) (*TrackerResponse, error) {
	// Try all trackers until one succeeds
	trackers := t.GetAllTrackers()

	for _, trackerURL := range trackers {
		resp, err := tc.requestPeers(trackerURL, t, port, event)
		if err != nil {
			// Log error and try next tracker
			fmt.Printf("Failed to contact tracker %s: %v\n", trackerURL, err)
			continue
		}

		if resp.FailureReason != "" {
			// Log failure and try next tracker
			fmt.Printf("Tracker %s returned failure: %s\n", trackerURL, resp.FailureReason)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all trackers failed")
}

func (tc *TrackerClient) requestPeers(trackerURL string, t *torrent.TorrentFile, port int, event string) (*TrackerResponse, error) {
	parsedURL, err := url.Parse(trackerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid tracker URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "http", "https":
		return tc.requestHTTPTracker(trackerURL, t, port, event)
	case "udp":
		return tc.requestUDPTracker(trackerURL, t, port, event)
	default:
		return nil, fmt.Errorf("unsupported tracker protocol: %s", parsedURL.Scheme)
	}
}

// requestHTTPTracker sends an HTTP/HTTPS tracker request.
func (tc *TrackerClient) requestHTTPTracker(trackerURL string, t *torrent.TorrentFile, port int, event string) (*TrackerResponse, error) {
	req := TrackerRequest{
		InfoHash:   t.InfoHash,
		PeerID:     tc.peerID,
		Port:       port,
		Downloaded: 0, // Simplified: we don't track upload/download for basic client
		Left:       t.Info.GetTotalLength(),
		Event:      event,
		NumWant:    50, // Request up to 50 peers
		Key:        tc.key,
	}

	// Build query parameters
	params := url.Values{}
	params.Set("info_hash", string(req.InfoHash[:]))
	params.Set("peer_id", string(req.PeerID[:]))
	params.Set("port", strconv.Itoa(req.Port))
	params.Set("uploaded", "0") // Simplified: no upload tracking
	params.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	params.Set("left", strconv.FormatInt(req.Left, 10))
	params.Set("compact", "1")
	if req.Event != "" {
		params.Set("event", req.Event)
	}
	params.Set("numwant", strconv.Itoa(req.NumWant))
	params.Set("key", strconv.FormatUint(uint64(req.Key), 10))

	// Make request
	fullURL := trackerURL + "?" + params.Encode()
	resp, err := tc.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	// Parse response
	decoder := bencode.NewDecoder(resp.Body)
	data, err := decoder.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %w", err)
	}

	dict, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response is not a dictionary")
	}

	return tc.parseTrackerResponse(dict)
}

func (tc *TrackerClient) requestUDPTracker(trackerURL string, t *torrent.TorrentFile, port int, event string) (*TrackerResponse, error) {
	parsedURL, err := url.Parse(trackerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid UDP tracker URL: %w", err)
	}

	// Resolve address
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(parsedURL.Hostname(), parsedURL.Port()))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP connection: %w", err)
	}
	defer conn.Close()

	// Set timeout
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	// Step 1: Send connect request
	transactionID := make([]byte, 4)
	rand.Read(transactionID)

	connectReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connectReq[0:8], 0x41727101980) // Protocol ID
	binary.BigEndian.PutUint32(connectReq[8:12], 0)            // Action: connect
	copy(connectReq[12:16], transactionID)

	_, err = conn.Write(connectReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send connect request: %w", err)
	}

	// Receive connect response
	connectResp := make([]byte, 16)
	n, err := conn.Read(connectResp)
	if err != nil {
		return nil, fmt.Errorf("failed to receive connect response: %w", err)
	}
	if n != 16 {
		return nil, fmt.Errorf("invalid connect response length: %d", n)
	}

	// Verify response
	respAction := binary.BigEndian.Uint32(connectResp[0:4])
	respTransactionID := connectResp[4:8]
	if respAction != 0 || !bytes.Equal(respTransactionID, transactionID) {
		return nil, fmt.Errorf("invalid connect response")
	}

	connectionID := connectResp[8:16]

	// Step 2: Send announce request
	rand.Read(transactionID)

	eventNum := uint32(0)
	switch event {
	case "started":
		eventNum = 2
	case "completed":
		eventNum = 1
	case "stopped":
		eventNum = 3
	}

	announceReq := make([]byte, 98)
	copy(announceReq[0:8], connectionID)                                            // Connection ID
	binary.BigEndian.PutUint32(announceReq[8:12], 1)                                // Action: announce
	copy(announceReq[12:16], transactionID)                                         // Transaction ID
	copy(announceReq[16:36], t.InfoHash[:])                                         // Info hash
	copy(announceReq[36:56], tc.peerID[:])                                          // Peer ID
	binary.BigEndian.PutUint64(announceReq[56:64], 0)                               // Downloaded
	binary.BigEndian.PutUint64(announceReq[64:72], uint64(t.Info.GetTotalLength())) // Left
	binary.BigEndian.PutUint64(announceReq[72:80], 0)                               // Uploaded
	binary.BigEndian.PutUint32(announceReq[80:84], eventNum)                        // Event
	binary.BigEndian.PutUint32(announceReq[84:88], 0)                               // IP (default)
	binary.BigEndian.PutUint32(announceReq[88:92], tc.key)                          // Key
	binary.BigEndian.PutUint32(announceReq[92:96], 50)                              // Num want
	binary.BigEndian.PutUint16(announceReq[96:98], uint16(port))                    // Port

	_, err = conn.Write(announceReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send announce request: %w", err)
	}

	// Receive announce response
	announceResp := make([]byte, 1024) // Buffer for response
	n, err = conn.Read(announceResp)
	if err != nil {
		return nil, fmt.Errorf("failed to receive announce response: %w", err)
	}

	if n < 20 {
		return nil, fmt.Errorf("invalid announce response length: %d", n)
	}

	// Parse announce response
	respAction = binary.BigEndian.Uint32(announceResp[0:4])
	respTransactionID = announceResp[4:8]
	if respAction != 1 || !bytes.Equal(respTransactionID, transactionID) {
		return nil, fmt.Errorf("invalid announce response")
	}

	interval := binary.BigEndian.Uint32(announceResp[8:12])
	leechers := binary.BigEndian.Uint32(announceResp[12:16])
	seeders := binary.BigEndian.Uint32(announceResp[16:20])

	// Parse peers (compact format)
	peerData := announceResp[20:n]
	if len(peerData)%6 != 0 {
		return nil, fmt.Errorf("invalid peer data length: %d", len(peerData))
	}

	var peers []PeerInfo
	for i := 0; i < len(peerData); i += 6 {
		ip := net.IP(peerData[i : i+4])
		port := binary.BigEndian.Uint16(peerData[i+4 : i+6])
		peers = append(peers, PeerInfo{
			IP:   ip.String(),
			Port: int(port),
		})
	}

	return &TrackerResponse{
		Interval:   int64(interval),
		Complete:   int64(seeders),
		Incomplete: int64(leechers),
		Peers:      peers,
	}, nil
}

func (tc *TrackerClient) parseTrackerResponse(dict map[string]interface{}) (*TrackerResponse, error) {
	resp := &TrackerResponse{}

	// Check for failure
	if failureBytes, ok := dict["failure reason"].([]byte); ok {
		resp.FailureReason = string(failureBytes)
		return resp, nil
	}

	// Parse warning message
	if warningBytes, ok := dict["warning message"].([]byte); ok {
		resp.WarningMessage = string(warningBytes)
	}

	// Parse interval
	if interval, ok := dict["interval"].(int64); ok {
		resp.Interval = interval
	} else {
		return nil, fmt.Errorf("missing interval in tracker response")
	}

	// Parse optional min interval
	if minInterval, ok := dict["min interval"].(int64); ok {
		resp.MinInterval = minInterval
	}

	// Parse tracker ID
	if trackerIDBytes, ok := dict["tracker id"].([]byte); ok {
		resp.TrackerID = string(trackerIDBytes)
	}

	// Parse complete/incomplete
	if complete, ok := dict["complete"].(int64); ok {
		resp.Complete = complete
	}
	if incomplete, ok := dict["incomplete"].(int64); ok {
		resp.Incomplete = incomplete
	}

	// Parse peers
	if peersData, ok := dict["peers"]; ok {
		switch peers := peersData.(type) {
		case []byte:
			// Compact format (binary)
			err := tc.parseCompactPeers(peers, resp)
			if err != nil {
				return nil, fmt.Errorf("failed to parse compact peers: %w", err)
			}
		case []interface{}:
			// Dictionary format
			err := tc.parseDictionaryPeers(peers, resp)
			if err != nil {
				return nil, fmt.Errorf("failed to parse dictionary peers: %w", err)
			}
		default:
			return nil, fmt.Errorf("invalid peers format")
		}
	}

	return resp, nil
}

func (tc *TrackerClient) parseCompactPeers(data []byte, resp *TrackerResponse) error {
	if len(data)%6 != 0 {
		return fmt.Errorf("invalid compact peers length: %d", len(data))
	}

	for i := 0; i < len(data); i += 6 {
		ip := net.IP(data[i : i+4])
		port := binary.BigEndian.Uint16(data[i+4 : i+6])

		resp.Peers = append(resp.Peers, PeerInfo{
			IP:   ip.String(),
			Port: int(port),
		})
	}

	return nil
}

func (tc *TrackerClient) parseDictionaryPeers(peers []interface{}, resp *TrackerResponse) error {
	for _, peerInterface := range peers {
		peerDict, ok := peerInterface.(map[string]interface{})
		if !ok {
			continue
		}

		peer := PeerInfo{}

		// Parse peer ID
		if peerIDBytes, ok := peerDict["peer id"].([]byte); ok {
			peer.ID = peerIDBytes
		}

		// Parse IP
		if ipBytes, ok := peerDict["ip"].([]byte); ok {
			peer.IP = string(ipBytes)
		} else {
			continue // Skip peers without IP
		}

		// Parse port
		if port, ok := peerDict["port"].(int64); ok {
			peer.Port = int(port)
		} else {
			continue // Skip peers without port
		}

		resp.Peers = append(resp.Peers, peer)
	}

	return nil
}

// GetPeerID returns the client's peer ID
func (tc *TrackerClient) GetPeerID() [20]byte {
	return tc.peerID
}

// IsValidPeer checks if a peer address is valid
func IsValidPeer(peer PeerInfo) bool {
	// Basic validation
	if peer.IP == "" || peer.Port <= 0 || peer.Port > 65535 {
		return false
	}

	// Parse IP to check validity
	ip := net.ParseIP(peer.IP)
	if ip == nil {
		return false
	}

	// Skip localhost and private networks in production
	if ip.IsLoopback() {
		return false
	}

	return true
}

// FormatPeers returns a string representation of peers
func FormatPeers(peers []PeerInfo) string {
	if len(peers) == 0 {
		return "No peers"
	}

	var parts []string
	for i, peer := range peers {
		if i >= 10 { // Limit display to first 10 peers
			parts = append(parts, fmt.Sprintf("... and %d more", len(peers)-10))
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d", peer.IP, peer.Port))
	}

	return strings.Join(parts, ", ")
}
