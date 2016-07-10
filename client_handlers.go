package irc

// TODO: implement concept of stackable, orthogonal "handler sets" that can be
// used to compose bot behavior in a modular fashion. There would simply be a
// slice of handlers for each command and each handler set would be able to
// have its own state (e.g. via interface?)

import (
	"errors"
	"strings"
)

var clientHandlers = map[string]Handler{
	// server ping
	"PING": HandlerFunc(func(c *Client, m *Message) {
		var pingMessage string
		if len(m.Params) > 0 {
			pingMessage = m.Params[0]
		} else {
			pingMessage = m.Trailing
		}
		c.Command("PONG", []string{pingMessage})
	}),

	// someone's nick changed
	"NICK": HandlerFunc(func(c *Client, m *Message) {
		if m.From.Nick == c.Nick {
			c.Nick = m.Trailing
		}
	}),

	// someone's nick changed
	"ERROR": HandlerFunc(func(c *Client, m *Message) {
		c.err <- errors.New(m.Trailing)
	}),

	// nickname already in use
	"433": HandlerFunc(func(c *Client, m *Message) {
		c.Nick += "_"
		c.NICK(c.Nick)
	}),

	// available modes
	"004": HandlerFunc(func(c *Client, m *Message) {
		// <server_name> <version> <user_modes> <chan_modes>
	}),

	// server capabilities
	"005": HandlerFunc(func(c *Client, m *Message) {
		// http://www.irc.org/tech_docs/005.html
		//
		// :clarent.nc.us.irchighway.net
		//	005
		//	bully
		//	AWAYLEN=200
		//	CASEMAPPING=rfc1459
		//	CHANMODES=b,k,l,ACMNORSTcimnprstz
		//	CHANNELLEN=64
		//	CHANTYPES=#
		//	CHARSET=ascii
		//	ELIST=MU
		//	ESILENCE
		//	EXTBAN=,ACNORSTUcjmz
		//	FNC
		//	KICKLEN=255
		//	MAP
		//	MAXBANS=60
		//	:are supported by this server
		// :clarent.nc.us.irchighway.net
		//	005
		//	bully
		//	MAXCHANNELS=30
		//	MAXPARA=32
		//	MAXTARGETS=20
		//	MODES=20
		//	NAMESX
		//	NETWORK=irchighway
		//	NICKLEN=31
		//	PREFIX=(qaohv)~&@%+
		//	SILENCE=32
		//	SSL=199.241.190.42:6697
		//	STARTTLS
		//	STATUSMSG=~&@%+
		//	TOPICLEN=307
		//	:are supported by this server
		// :clarent.nc.us.irchighway.net
		//	005
		//	bully
		//	UHNAMES
		//	USERIP
		//	VBANLIST
		//	WALLCHOPS
		//	WALLVOICES
		//	:are supported by this server

		if c.caps == nil {
			c.caps = make(map[string]string, len(m.Params))
		}
		for _, param := range m.Params {
			if param == c.Nick {
				continue
			}

			pair := strings.SplitN(param, "=", 2)
			switch len(pair) {
			case 0:
				continue
			case 1:
				c.caps[pair[0]] = ""
			case 2:
				c.caps[pair[0]] = pair[1]
			}
		}
	}),

	// NAMES list
	// :server 353 <nick> = <chan> :<name> <name> <name> ...
	"353": HandlerFunc(func(c *Client, m *Message) {
		var (
			chanName = m.Params[len(m.Params)-1]
			thisChan = c.chanByName(chanName)
		)

		if thisChan == nil {
			return
		}

		if thisChan.namesChan == nil {
			thisChan.namesChan = make(chan struct{})
		}

		names := strings.Fields(m.Trailing)
		users := make([]*Hostmask, len(names))
		for i, name := range names {
			name = strings.TrimLeft(name, c.caps["STATUSMSG"])
			users[i] = &Hostmask{Nick: name}
		}

		if thisChan.gettingNames {
			thisChan.Users = append(thisChan.Users, users...)
		} else {
			thisChan.gettingNames = true
			thisChan.Users = users
		}
	}),

	// end of NAMES list
	"366": HandlerFunc(func(c *Client, m *Message) {
		var (
			chanName = m.Params[len(m.Params)-1]
			thisChan = c.chanByName(chanName)
		)

		if thisChan != nil {
			thisChan.gettingNames = false
			close(thisChan.namesChan)
		}
	}),
}
