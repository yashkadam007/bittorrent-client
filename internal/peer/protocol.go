package peer

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// MessageType represents the type of peer wire protocol message
type MessageType uint8

const (
	MsgChoke         MessageType = 0
	MsgUnchoke       MessageType = 1
	MsgInterested    MessageType = 2
	MsgNotInterested MessageType = 3
	MsgHave          MessageType = 4
	MsgBitfield      MessageType = 5
	MsgRequest       MessageType = 6
	MsgPiece         MessageType = 7
	MsgCancel        MessageType = 8
	MsgPort          MessageType = 9
)

// Message represents a peer wire protocol message
type Message struct {
	Type    MessageType
	Payload []byte
}

// Handshake represents the BitTorrent handshake
type Handshake struct {
	Protocol string
	Reserved [8]byte
	InfoHash [20]byte
	PeerID   [20]byte
}

// Connection represents a connection to a peer
type Connection struct {
	conn       net.Conn
	infoHash   [20]byte
	peerID     [20]byte
	remotePeerID [20]byte
	choked     bool
	choking    bool
	interested bool
	peerInterested bool
	bitfield   []byte
}

// NewConnection creates a new peer connection
func NewConnection(conn net.Conn, infoHash, peerID [20]byte) *Connection {
	return &Connection{
		conn:     conn,
		infoHash: infoHash,
		peerID:   peerID,
		choked:   true,
		choking:  true,
	}
}

// Connect establishes a connection to a peer
func Connect(addr string, infoHash, peerID [20]byte) (*Connection, error) {
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to peer: %w", err)
	}

	peerConn := NewConnection(conn, infoHash, peerID)
	
	// Perform handshake
	err = peerConn.performHandshake()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	return peerConn, nil
}

// performHandshake performs the BitTorrent handshake
func (c *Connection) performHandshake() error {
	// Create handshake
	handshake := Handshake{
		Protocol: "BitTorrent protocol",
		InfoHash: c.infoHash,
		PeerID:   c.peerID,
	}

	// Send handshake
	err := c.sendHandshake(handshake)
	if err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Receive handshake
	remoteHandshake, err := c.receiveHandshake()
	if err != nil {
		return fmt.Errorf("failed to receive handshake: %w", err)
	}

	// Verify handshake
	if remoteHandshake.InfoHash != c.infoHash {
		return fmt.Errorf("info hash mismatch")
	}

	c.remotePeerID = remoteHandshake.PeerID
	return nil
}

// sendHandshake sends a handshake message
func (c *Connection) sendHandshake(h Handshake) error {
	// Protocol length + protocol + reserved + info hash + peer ID
	buf := make([]byte, 1+len(h.Protocol)+8+20+20)
	
	offset := 0
	buf[offset] = byte(len(h.Protocol))
	offset++
	
	copy(buf[offset:], []byte(h.Protocol))
	offset += len(h.Protocol)
	
	copy(buf[offset:], h.Reserved[:])
	offset += 8
	
	copy(buf[offset:], h.InfoHash[:])
	offset += 20
	
	copy(buf[offset:], h.PeerID[:])

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(buf)
	return err
}

// receiveHandshake receives a handshake message
func (c *Connection) receiveHandshake() (*Handshake, error) {
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	
	// Read protocol length
	protocolLenBuf := make([]byte, 1)
	_, err := io.ReadFull(c.conn, protocolLenBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read protocol length: %w", err)
	}
	
	protocolLen := int(protocolLenBuf[0])
	if protocolLen != 19 {
		return nil, fmt.Errorf("invalid protocol length: %d", protocolLen)
	}

	// Read rest of handshake
	handshakeBuf := make([]byte, protocolLen+8+20+20)
	_, err = io.ReadFull(c.conn, handshakeBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake: %w", err)
	}

	handshake := &Handshake{
		Protocol: string(handshakeBuf[:protocolLen]),
	}
	
	offset := protocolLen
	copy(handshake.Reserved[:], handshakeBuf[offset:offset+8])
	offset += 8
	
	copy(handshake.InfoHash[:], handshakeBuf[offset:offset+20])
	offset += 20
	
	copy(handshake.PeerID[:], handshakeBuf[offset:offset+20])

	return handshake, nil
}

// SendMessage sends a message to the peer
func (c *Connection) SendMessage(msg Message) error {
	var buf []byte
	
	if msg.Type == 255 { // Keep-alive
		buf = make([]byte, 4)
		binary.BigEndian.PutUint32(buf, 0)
	} else {
		payloadLen := len(msg.Payload)
		buf = make([]byte, 4+1+payloadLen)
		
		binary.BigEndian.PutUint32(buf[0:4], uint32(1+payloadLen))
		buf[4] = byte(msg.Type)
		copy(buf[5:], msg.Payload)
	}

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(buf)
	return err
}

// ReceiveMessage receives a message from the peer
func (c *Connection) ReceiveMessage() (*Message, error) {
	c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	
	// Read message length
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(c.conn, lengthBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}
	
	length := binary.BigEndian.Uint32(lengthBuf)
	
	// Keep-alive message
	if length == 0 {
		return &Message{Type: 255}, nil
	}
	
	if length > 1<<17 { // 128KB max message size
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	// Read message type and payload
	msgBuf := make([]byte, length)
	_, err = io.ReadFull(c.conn, msgBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	msg := &Message{
		Type:    MessageType(msgBuf[0]),
		Payload: msgBuf[1:],
	}

	return msg, nil
}

// SendKeepAlive sends a keep-alive message
func (c *Connection) SendKeepAlive() error {
	return c.SendMessage(Message{Type: 255})
}

// SendChoke sends a choke message
func (c *Connection) SendChoke() error {
	c.choking = true
	return c.SendMessage(Message{Type: MsgChoke})
}

// SendUnchoke sends an unchoke message
func (c *Connection) SendUnchoke() error {
	c.choking = false
	return c.SendMessage(Message{Type: MsgUnchoke})
}

// SendInterested sends an interested message
func (c *Connection) SendInterested() error {
	c.interested = true
	return c.SendMessage(Message{Type: MsgInterested})
}

// SendNotInterested sends a not interested message
func (c *Connection) SendNotInterested() error {
	c.interested = false
	return c.SendMessage(Message{Type: MsgNotInterested})
}

// SendHave sends a have message
func (c *Connection) SendHave(pieceIndex int) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(pieceIndex))
	return c.SendMessage(Message{Type: MsgHave, Payload: payload})
}

// SendBitfield sends a bitfield message
func (c *Connection) SendBitfield(bitfield []byte) error {
	return c.SendMessage(Message{Type: MsgBitfield, Payload: bitfield})
}

// SendRequest sends a request message
func (c *Connection) SendRequest(pieceIndex, begin, length int) error {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(pieceIndex))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return c.SendMessage(Message{Type: MsgRequest, Payload: payload})
}

// SendPiece sends a piece message
func (c *Connection) SendPiece(pieceIndex, begin int, data []byte) error {
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(payload[0:4], uint32(pieceIndex))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	copy(payload[8:], data)
	return c.SendMessage(Message{Type: MsgPiece, Payload: payload})
}

// SendCancel sends a cancel message
func (c *Connection) SendCancel(pieceIndex, begin, length int) error {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(pieceIndex))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return c.SendMessage(Message{Type: MsgCancel, Payload: payload})
}

// HandleMessage processes a received message
func (c *Connection) HandleMessage(msg *Message) error {
	switch msg.Type {
	case MsgChoke:
		c.choked = true
	case MsgUnchoke:
		c.choked = false
	case MsgInterested:
		c.peerInterested = true
	case MsgNotInterested:
		c.peerInterested = false
	case MsgHave:
		if len(msg.Payload) != 4 {
			return fmt.Errorf("invalid have message length: %d", len(msg.Payload))
		}
		pieceIndex := binary.BigEndian.Uint32(msg.Payload)
		return c.handleHave(int(pieceIndex))
	case MsgBitfield:
		c.bitfield = make([]byte, len(msg.Payload))
		copy(c.bitfield, msg.Payload)
	case MsgRequest:
		if len(msg.Payload) != 12 {
			return fmt.Errorf("invalid request message length: %d", len(msg.Payload))
		}
		pieceIndex := binary.BigEndian.Uint32(msg.Payload[0:4])
		begin := binary.BigEndian.Uint32(msg.Payload[4:8])
		length := binary.BigEndian.Uint32(msg.Payload[8:12])
		return c.handleRequest(int(pieceIndex), int(begin), int(length))
	case MsgPiece:
		if len(msg.Payload) < 8 {
			return fmt.Errorf("invalid piece message length: %d", len(msg.Payload))
		}
		pieceIndex := binary.BigEndian.Uint32(msg.Payload[0:4])
		begin := binary.BigEndian.Uint32(msg.Payload[4:8])
		data := msg.Payload[8:]
		return c.handlePiece(int(pieceIndex), int(begin), data)
	case MsgCancel:
		if len(msg.Payload) != 12 {
			return fmt.Errorf("invalid cancel message length: %d", len(msg.Payload))
		}
		pieceIndex := binary.BigEndian.Uint32(msg.Payload[0:4])
		begin := binary.BigEndian.Uint32(msg.Payload[4:8])
		length := binary.BigEndian.Uint32(msg.Payload[8:12])
		return c.handleCancel(int(pieceIndex), int(begin), int(length))
	case 255: // Keep-alive
		// Do nothing for keep-alive
	default:
		// Unknown message type, ignore
	}
	return nil
}

// handleHave handles a have message
func (c *Connection) handleHave(pieceIndex int) error {
	// Expand bitfield if necessary
	byteIndex := pieceIndex / 8
	if byteIndex >= len(c.bitfield) {
		newBitfield := make([]byte, byteIndex+1)
		copy(newBitfield, c.bitfield)
		c.bitfield = newBitfield
	}
	
	// Set the bit for this piece
	bitIndex := uint(pieceIndex % 8)
	c.bitfield[byteIndex] |= (0x80 >> bitIndex)
	
	return nil
}

// handleRequest handles a request message
func (c *Connection) handleRequest(pieceIndex, begin, length int) error {
	// TODO: Implement piece serving logic
	// For now, just log the request
	fmt.Printf("Received request for piece %d, begin %d, length %d\n", pieceIndex, begin, length)
	return nil
}

// handlePiece handles a piece message
func (c *Connection) handlePiece(pieceIndex, begin int, data []byte) error {
	// TODO: Implement piece storage logic
	// For now, just log the piece
	fmt.Printf("Received piece %d, begin %d, length %d\n", pieceIndex, begin, len(data))
	return nil
}

// handleCancel handles a cancel message
func (c *Connection) handleCancel(pieceIndex, begin, length int) error {
	// TODO: Implement request cancellation logic
	// For now, just log the cancellation
	fmt.Printf("Received cancel for piece %d, begin %d, length %d\n", pieceIndex, begin, length)
	return nil
}

// HasPiece returns true if the peer has the specified piece
func (c *Connection) HasPiece(pieceIndex int) bool {
	if c.bitfield == nil {
		return false
	}
	
	byteIndex := pieceIndex / 8
	if byteIndex >= len(c.bitfield) {
		return false
	}
	
	bitIndex := uint(pieceIndex % 8)
	return (c.bitfield[byteIndex] & (0x80 >> bitIndex)) != 0
}

// IsChoked returns true if this client is choked by the peer
func (c *Connection) IsChoked() bool {
	return c.choked
}

// IsChoking returns true if this client is choking the peer
func (c *Connection) IsChoking() bool {
	return c.choking
}

// IsInterested returns true if this client is interested in the peer
func (c *Connection) IsInterested() bool {
	return c.interested
}

// IsPeerInterested returns true if the peer is interested in this client
func (c *Connection) IsPeerInterested() bool {
	return c.peerInterested
}

// GetBitfield returns the peer's bitfield
func (c *Connection) GetBitfield() []byte {
	if c.bitfield == nil {
		return nil
	}
	
	result := make([]byte, len(c.bitfield))
	copy(result, c.bitfield)
	return result
}

// GetRemotePeerID returns the remote peer's ID
func (c *Connection) GetRemotePeerID() [20]byte {
	return c.remotePeerID
}

// Close closes the connection
func (c *Connection) Close() error {
	return c.conn.Close()
}

// String returns a string representation of the message type
func (m MessageType) String() string {
	switch m {
	case MsgChoke:
		return "choke"
	case MsgUnchoke:
		return "unchoke"
	case MsgInterested:
		return "interested"
	case MsgNotInterested:
		return "not_interested"
	case MsgHave:
		return "have"
	case MsgBitfield:
		return "bitfield"
	case MsgRequest:
		return "request"
	case MsgPiece:
		return "piece"
	case MsgCancel:
		return "cancel"
	case MsgPort:
		return "port"
	default:
		if m == 255 {
			return "keep_alive"
		}
		return fmt.Sprintf("unknown(%d)", m)
	}
}
