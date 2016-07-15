package irc

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"ktkr.us/pkg/irc/ratelimit"
)

// Client contains all of the state required by an event-driven IRC client.
type Client struct {
	Addr     string
	Nick     string
	User     string
	Realname string
	Pass     string
	Secure   bool // true to attempt TLS
	Verbose  bool

	conn net.Conn

	send     chan *Message
	recv     chan *Message
	err      chan error
	handlers map[string][]Handler
	l        *ratelimit.Limiter

	chans []*Channel
	caps  map[string]string
}

var (
	errEmptyNick = errors.New("irc: cannot use empty nick")
	errEmptyUser = errors.New("irc: cannot use empty username")
)

// Connect logs c into the configured host.
func (c *Client) Connect() error {
	if c.Nick == "" {
		return errEmptyNick
	}
	if c.User == "" {
		return errEmptyUser
	}

	var (
		conn net.Conn
		err  error
	)

	conn, err = net.Dial("tcp", c.Addr)
	if err != nil {
		return err
	}

	if c.Secure {
		//host, _, _ := net.SplitHostPort(c.Addr)
		tlsConf := tls.Config{InsecureSkipVerify: true}
		tlsConn := tls.Client(conn, &tlsConf)
		err = tlsConn.Handshake()
		if err != nil {
			return err
		}

		conn = tlsConn
	}

	c.conn = conn
	c.send = make(chan *Message, 10)
	c.recv = make(chan *Message, 10)
	c.err = make(chan error)
	c.l = ratelimit.New(time.Second, 4)

	c.Stack(defaultHandlers)

	firstLineCh := make(chan struct{})

	go c.recvLoop(firstLineCh)
	go c.sendLoop()

	// wait until we recieve the first data from the server to begin sending
	// commands
	<-firstLineCh

	if c.Pass != "" {
		c.PASS(c.Pass)
	}
	c.USER(c.User, c.Realname, 0)
	c.NICK(c.Nick)

	return nil
}

// recvLoop processes all network reads and handles incoming events.
func (c *Client) recvLoop(firstLineCh chan struct{}) {
	s := bufio.NewScanner(c.conn)
	for s.Scan() {
		if err := s.Err(); err != nil {
			c.err <- err
			break
		}
		if firstLineCh != nil {
			close(firstLineCh)
			firstLineCh = nil
		}

		line := s.Text()
		if c.Verbose {
			log.Println("<<", line)
		}

		m, err := ParseMessage(line, time.Now())
		if err != nil {
			log.Print("irc: client recv: %v", err)
			continue
		}

		if handlers, ok := c.handlers[m.Command]; ok {
			for _, h := range handlers {
				h.HandleIRC(c, m)
			}
		}
	}
}

// sendLoop gates all sends so chunks don't get interleaved accidentally when
// doing concurrent handlers. We can also do rate limiting here.
func (c *Client) sendLoop() {
	for {
		c.l.GrabTicket()
		m := <-c.send
		s := m.String()
		if c.Verbose {
			log.Println(">>", s)
		}
		io.WriteString(c.conn, s+"\r\n")
	}
}

// Handle adds a Handler to run when `cmd` is received.
func (c *Client) Handle(cmd string, h Handler) {
	if c.handlers == nil {
		c.handlers = map[string][]Handler{cmd: []Handler{h}}
	} else {
		if c.handlers[cmd] == nil {
			c.handlers[cmd] = []Handler{h}
		} else {
			c.handlers[cmd] = append(c.handlers[cmd], h)
		}
	}
}

// HandleFunc adds a HandlerFunc to run when `cmd` is received.
func (c *Client) HandleFunc(cmd string, handler HandlerFunc) {
	c.Handle(cmd, handler)
}

// Run handles events and blocks until the connection is closed
func (c *Client) Run() error {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	for {
		select {
		case err := <-c.err:
			return err
		case <-ch:
			return nil
		}
	}
}

// Stack appends handlers from hs to c.
func (c *Client) Stack(hs HandlerSet) {
	for k, v := range hs {
		c.Handle(k, v)
	}
}

// chanByName returns the Channel object corresponding to channel named `name`,
// or nil if c isn't joined to it.
func (c *Client) chanByName(name string) *Channel {
	for _, ch := range c.chans {
		if ch.Name == name {
			return ch
		}
	}
	return nil
}
