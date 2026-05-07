// tpt-payments/backend/internal/acquiring/connection_manager.go
package acquiring

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ConnectionManager manages persistent TCP connections to card network hosts
type ConnectionManager struct {
	connections map[string]*PersistentConnection
	mu          sync.RWMutex
	config      ConnectionConfig
	logger      *logrus.Logger
}

// ConnectionConfig holds configuration for TCP connections
type ConnectionConfig struct {
	ConnectTimeout   time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	KeepAlive       time.Duration
	MaxIdleConns    int
	MaxIdleConnsPerHost int
	IdleTimeout     time.Duration
	TLSConfig       *tls.Config
	HeartbeatInterval time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
}

// PersistentConnection represents a managed TCP connection
type PersistentConnection struct {
	Host       string
	Conn       net.Conn
	LastUsed   time.Time
	InUse      bool
	mu         sync.RWMutex
	cancel     context.CancelFunc
	ctx        context.Context
	heartbeat *time.Ticker
}

// DefaultConnectionConfig returns sensible defaults for connection management
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		ConnectTimeout:       30 * time.Second,
		ReadTimeout:          30 * time.Second,
		WriteTimeout:         30 * time.Second,
		KeepAlive:             60 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost:   10,
		IdleTimeout:           90 * time.Second,
		HeartbeatInterval:    30 * time.Second,
		MaxRetries:          3,
		RetryDelay:            5 * time.Second,
	}
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config ConnectionConfig, logger *logrus.Logger) *ConnectionManager {
	if logger == nil {
		logger = logrus.New()
	}
	return &ConnectionManager{
		connections: make(map[string]*PersistentConnection),
		config:      config,
		logger:      logger,
	}
}

// GetConnection returns a managed connection to the specified host
func (cm *ConnectionManager) GetConnection(ctx context.Context, host string) (net.Conn, error) {
	cm.mu.RLock()
	conn, exists := cm.connections[host]
	cm.mu.RUnlock()

	if exists && conn.IsHealthy() {
		conn.mu.Lock()
		conn.InUse = true
		conn.LastUsed = time.Now()
		conn.mu.Unlock()
		return &wrappedConn{conn: conn.Conn, pconn: conn}, nil
	}

	// Create new connection
	return cm.createConnection(ctx, host)
}

// createConnection creates a new persistent connection
func (cm *ConnectionManager) createConnection(ctx context.Context, host string) (net.Conn, error) {
	var lastErr error
	for attempt := 0; attempt <= cm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(cm.config.RetryDelay)
		}

		conn, err := cm.dialConnection(ctx, host)
		if err != nil {
			lastErr = err
			cm.logger.WithFields(logrus.Fields{
				"host":   host,
				"attempt": attempt + 1,
				"error":  err,
			}).Warn("Failed to connect, retrying")
			continue
		}

		// Create persistent connection wrapper
		ctx, cancel := context.WithCancel(context.Background())
		pconn := &PersistentConnection{
			Host:      host,
			Conn:      conn,
			LastUsed:  time.Now(),
			InUse:     true,
			ctx:       ctx,
			cancel:    cancel,
		}

		// Start heartbeat if configured
		if cm.config.HeartbeatInterval > 0 {
			pconn.heartbeat = time.NewTicker(cm.config.HeartbeatInterval)
			go cm.heartbeatLoop(pconn)
		}

		cm.mu.Lock()
		cm.connections[host] = pconn
		cm.mu.Unlock()

		return &wrappedConn{conn: conn, pconn: pconn}, nil
	}

	return nil, fmt.Errorf("failed to connect to %s after %d attempts: %w", host, cm.config.MaxRetries+1, lastErr)
}

// dialConnection establishes a TCP connection with timeout and TLS support
func (cm *ConnectionManager) dialConnection(ctx context.Context, host string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   cm.config.ConnectTimeout,
		KeepAlive: cm.config.KeepAlive,
	}

	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, fmt.Errorf("TCP dial failed: %w", err)
	}

	// Apply TLS if configured
	if cm.config.TLSConfig != nil {
		tlsConn := tls.Client(conn, cm.config.TLSConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}
		return tlsConn, nil
	}

	return conn, nil
}

// heartbeatLoop sends periodic heartbeats to keep the connection alive
func (cm *ConnectionManager) heartbeatLoop(pconn *PersistentConnection) {
	for {
		select {
		case <-pconn.ctx.Done():
			return
		case <-pconn.heartbeat.C:
			if !pconn.sendHeartbeat() {
				cm.logger.WithField("host", pconn.Host).Warn("Heartbeat failed, connection may be dead")
			}
		}
	}
}

// sendHeartbeat sends a heartbeat message (ISO 8583 echo/health check)
func (pconn *PersistentConnection) sendHeartbeat() bool {
	pconn.mu.RLock()
	defer pconn.mu.RUnlock()

	if pconn.Conn == nil {
		return false
	}

	// Send a simple heartbeat (network-specific)
	// For ISO 8583, this would be a Network Management Request (MTI 0800)
	heartbeatMsg := []byte{0x00, 0x00, 0x00, 0x00} // Simplified
	_ = pconn.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := pconn.Conn.Write(heartbeatMsg)
	return err == nil
}

// IsHealthy checks if the connection is still usable
func (pconn *PersistentConnection) IsHealthy() bool {
	pconn.mu.RLock()
	defer pconn.mu.RUnlock()

	if pconn.Conn == nil {
		return false
	}

	// Check if connection is closed
	if netConn, ok := pconn.Conn.(interface{ Close() error }); ok {
		_ = netConn
	}

	// Simple health check: try to read with zero timeout to detect EOF
	pconn.Conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 1)
	n, err := pconn.Conn.Read(buf)
	if n == 0 && err != nil {
		return false
	}

	return true
}

// ReleaseConnection marks a connection as no longer in use
func (cm *ConnectionManager) ReleaseConnection(host string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[host]; exists {
		conn.mu.Lock()
		conn.InUse = false
		conn.LastUsed = time.Now()
		conn.mu.Unlock()
	}
}

// CloseConnection closes a specific connection
func (cm *ConnectionManager) CloseConnection(host string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[host]; exists {
		conn.cancel()
		if conn.heartbeat != nil {
			conn.heartbeat.Stop()
		}
		conn.Conn.Close()
		delete(cm.connections, host)
		cm.logger.WithField("host", host).Info("Connection closed")
	}
}

// CloseAll closes all managed connections
func (cm *ConnectionManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for host, conn := range cm.connections {
		conn.cancel()
		if conn.heartbeat != nil {
			conn.heartbeat.Stop()
		}
		conn.Conn.Close()
		delete(cm.connections, host)
	}
	cm.logger.Info("All connections closed")
}

// CleanupIdle removes connections that have been idle for too long
func (cm *ConnectionManager) CleanupIdle() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for host, conn := range cm.connections {
		conn.mu.RLock()
		idle := !conn.InUse && now.Sub(conn.LastUsed) > cm.config.IdleTimeout
		conn.mu.RUnlock()

		if idle {
			conn.cancel()
			if conn.heartbeat != nil {
				conn.heartbeat.Stop()
			}
			conn.Conn.Close()
			delete(cm.connections, host)
			cm.logger.WithField("host", host).Info("Idle connection cleaned up")
		}
	}
}

// wrappedConn wraps a net.Conn and tracks when it's released
type wrappedConn struct {
	conn  net.Conn
	pconn *PersistentConnection
}

func (wc *wrappedConn) Read(b []byte) (n int, err error) {
	return wc.conn.Read(b)
}

func (wc *wrappedConn) Write(b []byte) (n int, err error) {
	return wc.conn.Write(b)
}

func (wc *wrappedConn) Close() error {
	wc.pconn.LastUsed = time.Now()
	wc.pconn.InUse = false
	// Don't actually close the underlying connection
	return nil
}

func (wc *wrappedConn) LocalAddr() net.Addr {
	return wc.conn.LocalAddr()
}

func (wc *wrappedConn) RemoteAddr() net.Addr {
	return wc.conn.RemoteAddr()
}

func (wc *wrappedConn) SetDeadline(t time.Time) error {
	return wc.conn.SetDeadline(t)
}

func (wc *wrappedConn) SetReadDeadline(t time.Time) error {
	return wc.conn.SetReadDeadline(t)
}

func (wc *wrappedConn) SetWriteDeadline(t time.Time) error {
	return wc.conn.SetWriteDeadline(t)
}

// GetConnectionStats returns statistics about managed connections
func (cm *ConnectionManager) GetConnectionStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := map[string]interface{}{
		"total_connections": len(cm.connections),
		"hosts":             make([]map[string]interface{}, 0),
	}

	hosts := stats["hosts"].([]map[string]interface{})
	for host, conn := range cm.connections {
		conn.mu.RLock()
		hosts = append(hosts, map[string]interface{}{
			"host":      host,
			"in_use":    conn.InUse,
			"last_used": conn.LastUsed,
		})
		conn.mu.RUnlock()
	}
	stats["hosts"] = hosts

	return stats
}

// ParseHost extracts host:port from a URL string
func ParseHost(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("no host in endpoint URL")
	}
	return u.Host, nil
}