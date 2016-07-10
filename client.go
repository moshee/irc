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
	handlers map[string]Handler
	l        *ratelimit.Limiter

	chans []*Channel
	caps  map[string]string
}

var (
	errEmptyNick = errors.New("irc: cannot use empty nick")
	errEmptyUser = errors.New("irc: cannot use empty username")
)

func (c *Client) Connect() error {
	if c.Nick == "" {
		return errEmptyNick
	}
	if c.User == "" {
		return errEmptyUser
	}

	conn, err := net.Dial("tcp", c.Addr)
	if err != nil {
		return err
	}

	if c.Secure {
		host, _, _ := net.SplitHostPort(c.Addr)
		tlsConf := tls.Config{ServerName: host}
		conn = tls.Client(conn, &tlsConf)
	}

	c.conn = conn
	c.send = make(chan *Message, 10)
	c.recv = make(chan *Message, 10)
	c.err = make(chan error)
	c.l = ratelimit.New(time.Second, 4)

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

		// user handlers override built-in ones
		if handler, ok := c.handlers[m.Command]; ok && handler != nil {
			handler.HandleIRC(c, m)
		} else if handler, ok = clientHandlers[m.Command]; ok && handler != nil {
			handler.HandleIRC(c, m)
		}
	}
}

// Gate all sends so chunks don't get interleaved accidentally when doing
// concurrent handlers. We can also do rate limiting here.
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

func (c *Client) Handle(command string, handler Handler) {
	if c.handlers == nil {
		c.handlers = map[string]Handler{command: handler}
	} else {
		c.handlers[command] = handler
	}
}

func (c *Client) HandleFunc(command string, handler HandlerFunc) {
	c.Handle(command, handler)
}

// blocks until the connection is closed
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

func (c *Client) chanByName(name string) *Channel {
	for _, ch := range c.chans {
		if ch.Name == name {
			return ch
		}
	}
	return nil
}
