package irc

import (
	"log"
	"strconv"
	"strings"
)

// SendRaw sends a raw command string to the remote server.
func (c *Client) SendRaw(s string) {
	m, err := ParseMessage(s)
	if err != nil {
		log.Printf("malformed command: %s (%v)", s, err)
		return
	}

	c.send <- m
}

// Command sends a well-formed command to the remote server.
func (c *Client) Command(cmd string, params []string, trailing ...string) {
	m := &Message{
		Command: cmd,
		Params:  params,
	}
	if len(trailing) > 0 {
		m.Trailing = strings.Join(trailing, " ")
	}
	c.send <- m
}

func (c *Client) PASS(pass string) {
	c.Command("PASS", []string{pass})
}

func (c *Client) USER(user, realname string, modes int) {
	c.Command("USER", []string{user, strconv.Itoa(modes), "*"}, realname)
}

func (c *Client) NICK(nick string) {
	c.Command("NICK", []string{nick})
}

func (c *Client) PRIVMSG(target, message string) {
	c.Command("PRIVMSG", []string{target}, message)
}

func (c *Client) JOIN(channel string, key ...string) {
	if len(key) > 0 {
		c.Command("JOIN", []string{channel, key[0]})
	} else {
		c.Command("JOIN", []string{channel})
	}
}
