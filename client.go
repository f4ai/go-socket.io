package socketio

import (
	"errors"
	"fmt"
	"github.com/f4ai/go-socket.io/engineio"
	"github.com/f4ai/go-socket.io/engineio/transport"
	"github.com/f4ai/go-socket.io/engineio/transport/polling"
	"github.com/f4ai/go-socket.io/engineio/transport/websocket"
	"github.com/f4ai/go-socket.io/logger"
	"github.com/f4ai/go-socket.io/parser"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

var EmptyAddrErr = errors.New("empty addr")

// Client is client for socket.io server
type Client struct {
	namespace string
	url       string

	conn     *conn
	handlers *namespaceHandlers

	opts    *engineio.Options
	backoff *BackOff
	lock    sync.Mutex

	reconnection         bool
	reconnecting         bool
	reconnectionAttempts float64
}

type ClientOptions struct {
	engineio.Options
	Transports           []transport.Transport
	Reconnection         bool
	ReconnectionDelay    float64
	ReconnectionDelayMax float64
	ReconnectionAttempts float64
}

// NewClient returns a server
// addr like http://asd.com:8080/{$namespace}
func NewClient(addr string, opts *ClientOptions) (*Client, error) {
	if addr == "" {
		return nil, EmptyAddrErr
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	namespace := fmtNS(u.Path)

	// Not allowing other than default
	u.Path = path.Join("/socket.io", namespace)
	u.Path = u.EscapedPath()
	if strings.HasSuffix(u.Path, "socket.io") {
		u.Path += "/"
	}
	// attempts
	attempts, _ := strconv.ParseFloat("Infinity", 64)
	if opts.ReconnectionAttempts > 0 {
		attempts = opts.ReconnectionAttempts
	}

	return &Client{
		namespace: namespace,
		url:       u.String(),
		handlers:  newNamespaceHandlers(),
		opts: &engineio.Options{
			Transports: opts.Transports,
		},
		backoff: NewBackOff(BackOff{
			ms:       opts.ReconnectionDelay,
			max:      opts.ReconnectionDelayMax,
			factor:   2,
			jitter:   0.5, // opts.randomizationFactor
			attempts: 0,
		}),
		reconnection:         opts.Reconnection,
		reconnecting:         false,
		reconnectionAttempts: attempts,
	}, err
}

func fmtNS(ns string) string {
	if ns == aliasRootNamespace {
		return rootNamespace
	}

	return ns
}

func (c *Client) ReConnection() error {
	return c.reconnect()
}

func (c *Client) reconnect() error {
	for {
		// reconnecting return
		if c.reconnecting {
			return nil
		}
		// reconnecting
		c.reconnecting = true
		// check attempts
		logger.Info("c.backoff.attempts", c.backoff.attempts)
		if c.backoff.attempts >= c.reconnectionAttempts {
			//c.backoff.Reset()
			c.reconnecting = false
			logger.Error("reconnect error", errors.New("reconnect failed: reconnect times more than backoff attempts"))
			break
		}
		// Duration delay
		delay := c.backoff.Duration()
		logger.Info(fmt.Sprintf("client will wait some %dms before reconnect attempt", time.Duration(delay)/time.Millisecond))
		time.Sleep(time.Duration(delay))
		// reconnect
		err := c.Connect()
		if err == nil {
			c.reconnecting = false
			break
		}
		logger.Error("reconnect failed: ", err)
		// reset
		c.reconnecting = false
	}
	return nil
}

func (c *Client) Connect() error {
	dialer := engineio.Dialer{
		Transports: []transport.Transport{polling.Default, websocket.Default},
	}
	// Use opts Transports when NewClient
	if c.opts != nil && len(c.opts.Transports) > 0 {
		dialer.Transports = c.opts.Transports
	}

	enginioCon, err := dialer.Dial(c.url, nil)
	if err != nil {
		return err
	}

	c.conn = newConn(enginioCon, c.handlers)

	if err := c.conn.connectClient(c); err != nil {
		_ = c.Close()
		if root, ok := c.handlers.Get(c.namespace); ok && root.onError != nil {
			root.onError(nil, err)
		}

		return err
	}

	go c.clientError()
	go c.clientWrite()
	go c.clientRead()

	return nil
}

// Close closes server.
func (c *Client) Close() error {
	err := c.conn.Close()
	//c.backoff.Reset()
	//if c.reconnection {
	//	return c.reconnect()
	//}
	//c.reconnecting = false
	return err
}

func (c *Client) Emit(event string, args ...interface{}) {
	nsConn, ok := c.conn.namespaces.Get(c.namespace)
	if !ok {
		logger.Info("Connection Namespace not initialized")
		return
	}

	nsConn.Emit(event, args...)
}

// OnConnect set a handler function f to handle open event for namespace.
func (c *Client) OnConnect(f func(Conn) error) {
	h := c.getNamespace(c.namespace)
	if h == nil {
		h = c.createNamespace(c.namespace)
	}

	h.OnConnect(f)
}

// OnDisconnect set a handler function f to handle disconnect event for namespace.
func (c *Client) OnDisconnect(f func(Conn, string)) {
	h := c.getNamespace(c.namespace)
	if h == nil {
		h = c.createNamespace(c.namespace)
	}

	h.OnDisconnect(func(cc Conn, s string) {
		// The library must have retry first
		if c.reconnection {
			err := c.reconnect()
			if err != nil {
				c.conn.onError(cc.Namespace(), err)
			}
		}
		// If cannot retry connect notify to handler
		f(cc, s)
	})

}

// OnError set a handler function f to handle error for namespace.
func (c *Client) OnError(f func(Conn, error)) {
	h := c.getNamespace(c.namespace)
	if h == nil {
		h = c.createNamespace(c.namespace)
	}

	h.OnError(f)
}

// OnEvent set a handler function f to handle event for namespace.
func (c *Client) OnEvent(event string, f interface{}) {
	h := c.getNamespace(c.namespace)
	if h == nil {
		h = c.createNamespace(c.namespace)
	}

	h.OnEvent(event, f)
}

func (c *Client) clientError() {
	defer func() {
		if err := c.Close(); err != nil {
			logger.Error("close connect:", err)
		}
	}()

	for {
		select {
		case <-c.conn.quitChan:
			return
		case err := <-c.conn.errorChan:
			logger.Error("clientError", err)

			var errMsg *errorMessage
			if !errors.As(err, &errMsg) {
				continue
			}

			if handler := c.conn.namespace(errMsg.namespace); handler != nil {
				if handler.onError != nil {
					nsConn, ok := c.conn.namespaces.Get(errMsg.namespace)
					if !ok {
						continue
					}
					handler.onError(nsConn, errMsg.err)
				}
			}
		}
	}
}

func (c *Client) clientWrite() {
	defer func() {
		if err := c.Close(); err != nil {
			logger.Error("close connect:", err)
		}

	}()

	for {
		select {
		case <-c.conn.quitChan:
			logger.Info("clientWrite Writer loop has stopped")
			return
		case pkg := <-c.conn.writeChan:
			if err := c.conn.encoder.Encode(pkg.Header, pkg.Data); err != nil {
				c.conn.onError(pkg.Header.Namespace, err)
			}
		}
	}
}

func (c *Client) clientRead() {
	defer func() {
		if err := c.Close(); err != nil {
			logger.Error("close connect:", err)
		}
	}()

	var event string

	for {
		var header parser.Header

		if err := c.conn.decoder.DecodeHeader(&header, &event); err != nil {
			c.conn.onError(c.namespace, err)

			logger.Error("clientRead Error in Decoder", err)

			return
		}

		if header.Namespace == aliasRootNamespace {
			header.Namespace = rootNamespace
		}

		var err error
		switch header.Type {
		case parser.Ack:
			err = ackPacketHandler(c.conn, header)
		case parser.Connect:
			err = clientConnectPacketHandler(c.conn, header)
		case parser.Disconnect:
			err = clientDisconnectPacketHandler(c.conn, header)
			return
		case parser.Event:
			err = eventPacketHandler(c.conn, event, header)
		default:

		}

		if err != nil {
			logger.Error("client read:", err)

			return
		}
	}
}

func (c *Client) createNamespace(ns string) *namespaceHandler {
	handler := newNamespaceHandler(ns, nil)
	c.handlers.Set(ns, handler)

	return handler
}

func (c *Client) getNamespace(ns string) *namespaceHandler {
	ret, ok := c.handlers.Get(ns)
	if !ok {
		return nil
	}

	return ret
}

func (c *conn) connectClient(client *Client) error {
	rootHandler, ok := c.handlers.Get(client.namespace)
	if !ok {
		return errUnavailableRootHandler
	}

	root := newNamespaceConn(c, client.namespace, rootHandler.broadcast)
	c.namespaces.Set(client.namespace, root)

	root.Join(root.Conn.ID())

	c.namespaces.Range(func(ns string, nc *namespaceConn) {
		nc.SetContext(c.Conn.Context())
	})

	header := parser.Header{
		Type:      parser.Connect,
		Namespace: client.namespace,
	}

	return c.encoder.Encode(header)
}
