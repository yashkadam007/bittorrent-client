package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yashkadam007/bittorrent-client/internal/download"
)

// Model represents the terminal UI state
type Model struct {
	// Download state
	torrentName     string
	totalSize       int64
	downloadManager *download.DownloadManager

	// UI state
	width      int
	height     int
	lastUpdate time.Time

	// Cached stats for display
	stats    download.DownloadStats
	progress ProgressInfo
	peers    []PeerInfo

	// UI flags
	showHelp bool
	quitting bool
}

// ProgressInfo holds download progress information
type ProgressInfo struct {
	CompletedPieces int
	TotalPieces     int
	Percentage      float64
	DownloadedBytes int64
	TotalBytes      int64
}

// PeerInfo holds information about connected peers
type PeerInfo struct {
	Address         string
	DownloadedBytes int64
	Status          string
}

// NewModel creates a new TUI model
func NewModel(torrentName string, totalSize int64, dm *download.DownloadManager) Model {
	return Model{
		torrentName:     torrentName,
		totalSize:       totalSize,
		downloadManager: dm,
		lastUpdate:      time.Now(),
		showHelp:        false,
		quitting:        false,
	}
}

// Init initializes the model (required by bubbletea)
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tickCmd(),
	)
}

// Update handles incoming messages (required by bubbletea)
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "h", "?":
			m.showHelp = !m.showHelp
			return m, nil
		}

	case tickMsg:
		// Update stats from download manager
		m.updateStats()
		return m, tickCmd()

	case completionMsg:
		// Download completed
		m.progress.Percentage = 100.0
		m.progress.CompletedPieces = m.progress.TotalPieces
		return m, nil

	case tea.QuitMsg:
		return m, tea.Quit
	}

	return m, nil
}

// View renders the UI (required by bubbletea)
func (m Model) View() string {
	if m.quitting {
		return "Thanks for using BitTorrent Client!\n"
	}

	if m.showHelp {
		return m.helpView()
	}

	return m.mainView()
}

// updateStats fetches latest stats from download manager
func (m *Model) updateStats() {
	if m.downloadManager == nil {
		return
	}

	// Get download statistics
	m.stats = m.downloadManager.GetStats()

	// Get progress information
	completed, total, percentage := m.downloadManager.GetProgress()
	m.progress = ProgressInfo{
		CompletedPieces: completed,
		TotalPieces:     total,
		Percentage:      percentage,
		DownloadedBytes: m.stats.DownloadedBytes,
		TotalBytes:      m.totalSize,
	}

	m.lastUpdate = time.Now()
}

// mainView renders the main download interface
func (m Model) mainView() string {
	var sections []string

	// Header
	sections = append(sections, m.headerView())

	// Progress section
	sections = append(sections, m.progressView())

	// Stats section
	sections = append(sections, m.statsView())

	// Piece visualization
	sections = append(sections, m.pieceView())

	// Footer
	sections = append(sections, m.footerView())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// headerView renders the header with torrent info
func (m Model) headerView() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED")).
		Render("🌊 BitTorrent Client")

	name := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#059669")).
		Render(m.torrentName)

	return fmt.Sprintf("%s\n%s\n", title, name)
}

// progressView renders the download progress bar
func (m Model) progressView() string {
	if m.width < 40 {
		return "Terminal too small\n"
	}

	progressWidth := m.width - 20
	if progressWidth > 60 {
		progressWidth = 60
	}

	completed := int(float64(progressWidth) * (m.progress.Percentage / 100))
	remaining := progressWidth - completed

	progressBar := strings.Repeat("█", completed) + strings.Repeat("░", remaining)

	progressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	percentage := fmt.Sprintf("%.1f%%", m.progress.Percentage)

	return fmt.Sprintf("\n📥 Download Progress:\n%s %s\n",
		progressStyle.Render(progressBar), percentage)
}

// statsView renders download statistics
func (m Model) statsView() string {
	_ = time.Since(m.stats.StartTime) // For potential future use

	// Format file sizes
	downloadedSize := formatBytes(m.progress.DownloadedBytes)
	totalSize := formatBytes(m.progress.TotalBytes)

	// Format speed
	speed := formatSpeed(m.stats.DownloadSpeed)

	// Calculate ETA
	eta := "∞"
	if m.stats.DownloadSpeed > 0 {
		remaining := float64(m.progress.TotalBytes - m.progress.DownloadedBytes)
		etaSeconds := remaining / m.stats.DownloadSpeed
		eta = formatDuration(time.Duration(etaSeconds) * time.Second)
	}

	statsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6366F1"))

	return fmt.Sprintf("\n📊 Statistics:\n%s\n%s\n%s\n%s\n%s\n",
		statsStyle.Render(fmt.Sprintf("Size:      %s / %s", downloadedSize, totalSize)),
		statsStyle.Render(fmt.Sprintf("Pieces:    %d / %d", m.progress.CompletedPieces, m.progress.TotalPieces)),
		statsStyle.Render(fmt.Sprintf("Speed:     %s", speed)),
		statsStyle.Render(fmt.Sprintf("Peers:     %d", m.stats.PeersConnected)),
		statsStyle.Render(fmt.Sprintf("ETA:       %s", eta)),
	)
}

// pieceView renders piece completion visualization
func (m Model) pieceView() string {
	if m.progress.TotalPieces == 0 {
		return ""
	}

	// Limit visualization to reasonable size
	maxPieces := 100
	displayPieces := m.progress.TotalPieces
	if displayPieces > maxPieces {
		displayPieces = maxPieces
	}

	// Calculate pieces per display unit
	piecesPerUnit := float64(m.progress.TotalPieces) / float64(displayPieces)

	var pieces []string
	for i := 0; i < displayPieces; i++ {
		startPiece := int(float64(i) * piecesPerUnit)
		_ = int(float64(i+1) * piecesPerUnit) // endPiece for potential future use

		// Check if all pieces in this range are complete
		// For simplification, we'll use overall completion percentage
		completed := float64(startPiece) < float64(m.progress.CompletedPieces)

		if completed {
			pieces = append(pieces, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")).
				Render("█"))
		} else {
			pieces = append(pieces, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")).
				Render("░"))
		}
	}

	// Break into multiple lines if too wide
	piecesPerLine := 50
	var lines []string
	for i := 0; i < len(pieces); i += piecesPerLine {
		end := i + piecesPerLine
		if end > len(pieces) {
			end = len(pieces)
		}
		lines = append(lines, strings.Join(pieces[i:end], ""))
	}

	return fmt.Sprintf("\n🧩 Pieces:\n%s\n", strings.Join(lines, "\n"))
}

// footerView renders the footer with help info
func (m Model) footerView() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).
		Italic(true)

	return fmt.Sprintf("\n%s\n",
		helpStyle.Render("Press 'h' for help • 'q' to quit"))
}

// helpView renders the help screen
func (m Model) helpView() string {
	helpStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1)

	help := `🌊 BitTorrent Client - Help

Keyboard Controls:
  h, ?    Toggle this help screen
  q       Quit the application
  Ctrl+C  Force quit

Information Display:
  📥 Progress bar shows download completion
  📊 Statistics show speed, peers, and ETA
  🧩 Piece visualization shows which parts are complete

The client automatically:
  • Connects to peers from trackers
  • Downloads pieces using rarest-first strategy
  • Verifies pieces with SHA1 hashes
  • Assembles files on disk

Press 'h' again to return to the main view.`

	return helpStyle.Render(help)
}

// Utility functions for formatting

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatSpeed(bytesPerSecond float64) string {
	const (
		KB = 1024
		MB = KB * 1024
	)

	switch {
	case bytesPerSecond >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSecond/MB)
	case bytesPerSecond >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSecond/KB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "∞"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

// tickMsg is sent periodically to update the display
type tickMsg time.Time

// completionMsg is sent when download completes
type completionMsg struct{}

// tickCmd returns a command that sends a tick message every second
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
