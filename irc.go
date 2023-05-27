package irc

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"
)

var (
	errHostmaskEmpty   = errors.New("empty hostmask")
	errHostmaskInvalid = errors.New("malformed hostmask")
	errMessageEmpty    = errors.New("empty message")
	errMessageInvalid  = errors.New("malformed message")
)

type Hostmask struct {
	Nick    string
	User    string
	Address string
}

func ParseHostmask(s string) (*Hostmask, error) {
	if len(s) == 0 {
		return nil, errHostmaskEmpty
	}

	bang := strings.Index(s, "!")
	at := strings.Index(s, "@")

	if bang == -1 && at == -1 {
		return &Hostmask{Address: s}, nil
	}

	// empty nick or no bang || no at || bang happens after at or user is empty
	if bang < 1 || at < 0 || bang > (at-2) {
		return nil, errHostmaskInvalid
	}

	return &Hostmask{s[:bang], s[bang+1 : at], s[at+1:]}, nil
}

func (h *Hostmask) String() string {
	if h.Nick == "" && h.User == "" {
		return h.Address
	}
	return fmt.Sprintf("%s!%s@%s", h.Nick, h.User, h.Address)
}

func (h *Hostmask) MatchString(other string) bool {
	otherHost, err := ParseHostmask(other)
	if err != nil {
		return false
	}
	if *otherHost == *h {
		return true
	}
	return h.Match(otherHost)
}

func (h *Hostmask) Match(other *Hostmask) bool {
	/*
		return MatchPattern(h.Nick, other.Nick) &&
			MatchPattern(h.User, other.User) &&
			MatchPattern(h.Address, other.Address)
	*/
	return true
}

// MatchPattern performs a case-insensitive fuzzy string comparison using the
// following syntax:
//     - * matches 0 or more characters
//     - ? matches exactly one character
func MatchPattern(pattern, s string, t *testing.T) bool {
	t.Log()
	t.Logf("%q <=> %q\n", pattern, s)

	if len(pattern) == 0 {
		t.Log("bail false: empty pattern matches nothing")
		return false
	}

	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)

	// might match erroneously?
	if pattern == s {
		t.Log("bail true: strings are equal")
		return true
	}

	var (
		i, j           int
		a, b, nextChar rune
		patternRunes   = []rune(pattern)
		sRunes         = []rune(s)
	)

	// guarantees:
	// - len(pattern) > 0
	// - pattern != s
	for {
		/*
			sEnd	patternEnd	result
			0		0			keep going
			0		1			fail - string has too many chars
			1		0			fail - pattern wants to match more chars
			1		1			done
		*/
		sEnd := i > len(sRunes)-1
		patternEnd := j > len(patternRunes)-1

		t.Logf("sEnd = %t, patternEnd = %t\n", sEnd, patternEnd)

		if sEnd == patternEnd {
			if sEnd {
				t.Log("finished: whole pattern match")
				break
			}
		} else if patternEnd {
			t.Log("bail false: string has too many chars")
			return false
		}

		b = patternRunes[j]

		if sEnd && b != '*' {
			t.Log("bail false: pattern has too many chars")
			return false
		}

		//           0123  len = 4
		// pattern: "...*"
		//              ^
		//              j = 3
		// condition true if we see * and are at the end of the string
		if b == '*' && j >= len(patternRunes)-1 {
			// anything beyond the end will match
			t.Log("bail true: catch-all end splat")
			return true
		}

		a = sRunes[i]

		// check for next character after splat matching
		if b == '*' {
			nextChar = patternRunes[j+1]
		}

		match, advance := matchChar(a, b, nextChar)

		t.Logf("matchChar(%q, %q, %q) match=%t advance=%t\n", a, b, nextChar, match, advance)

		if !match {
			t.Log("bail false: unmatched char")
			return false
		}

		// aa*ba
		// aa12345ba
		// aa******a
		// ttffffftt

		if advance {
			j++
			if b == '*' {
				j++
			}
		}

		i++

	}

	return true
}

func matchChar(a, b, next rune) (match, advance bool) {
	switch b {
	case '*':
		return true, a == next
	case '?':
		return true, true
	default:
		return unicode.ToLower(a) == unicode.ToLower(b), true
	}
}

type MessageTarget interface {
	Name() string
}

type (
	NoTarget      struct{}
	ChannelTarget string
	UserTarget    string
)

func (t NoTarget) Name() string      { return "" }
func (t ChannelTarget) Name() string { return string(t) }
func (t UserTarget) Name() string    { return string(t) }

type Message struct {
	Time     time.Time
	From     *Hostmask
	Command  string
	Params   []string
	Trailing string
}

func (m *Message) String() string {
	s := ""
	if m.From != nil {
		s += ":" + m.From.String() + " "
	}
	if m.Command != "" {
		s += m.Command
	}
	if m.Params != nil && len(m.Params) > 0 {
		s += " " + strings.Join(m.Params, " ")
	}
	if m.Trailing != "" {
		s += " :" + m.Trailing
	}
	return s
}

// parse message with optional timestamp
func ParseMessage(s string, t ...time.Time) (*Message, error) {
	/*
			RFC2812 §2.3.1
		    message    =  [ ":" prefix SPACE ] command [ params ] crlf
		    prefix     =  servername / ( nickname [ [ "!" user ] "@" host ] )
		    command    =  1*letter / 3digit
		    params     =  *14( SPACE middle ) [ SPACE ":" trailing ]
		               =/ 14( SPACE middle ) [ SPACE [ ":" ] trailing ]

		    nospcrlfcl =  %x01-09 / %x0B-0C / %x0E-1F / %x21-39 / %x3B-FF
		                    ; any octet except NUL, CR, LF, " " and ":"
		    middle     =  nospcrlfcl *( ":" / nospcrlfcl )
		    trailing   =  *( ":" / " " / nospcrlfcl )

		    SPACE      =  %x20        ; space character
		    crlf       =  %x0D %x0A   ; "carriage return" "linefeed"
	*/

	if len(s) == 0 {
		return nil, errMessageEmpty
	}

	m := &Message{}
	if len(t) > 0 {
		m.Time = t[0]
	}

	s = strings.TrimSpace(s)

	// prefix
	if s[0] == ':' {
		i := strings.Index(s, " ")
		if i < 0 {
			// host only and nothing else? pretty weird
			return nil, errMessageInvalid
		}
		hs := s[1:i]
		s = s[i+1:]

		h, err := ParseHostmask(hs)
		if err != nil {
			return nil, err
		}

		m.From = h
	}

	s = strings.TrimSpace(s)
	if m.From != nil && s == "" {
		return nil, errMessageInvalid
	}

	// command
	i := strings.Index(s, " ")
	if i < 0 {
		// no space means the command is the last element
		m.Command = strings.ToUpper(s)
		return m, nil
	} else {
		m.Command = strings.ToUpper(s[:i])
		s = s[i+1:]
	}

	// params or trailing

	for {
		if len(s) == 0 {
			return m, nil
		}
		switch s[0] {
		case ' ':
			s = s[1:]
		case ':':
			m.Trailing = s[1:]
			return m, nil
		default:
			if len(m.Params) == 14 {
				// Max 14 params. If there are 14 then a colon isn't required
				// for the trailing. So we just pack up the rest and leave once
				// we reach 14 params.
				m.Trailing = s
				return m, nil
			}
			i = strings.Index(s, " ")
			if i < 0 {
				m.Params = append(m.Params, s)
				return m, nil
			}
			m.Params = append(m.Params, s[:i])
			s = s[i+1:]
		}
	}

	panic("unreachable")
}

func (m *Message) Target() MessageTarget {
	if len(m.Params) == 0 || m.Params[0] == "" {
		return NoTarget{}
	}

	switch m.Params[0][0] {
	case '#', '&', '+', '!':
		return ChannelTarget(m.Params[0])
	default:
		return UserTarget(m.Params[0])
	}
}

/*
// map[mirc]ansi256
colormap := []int{ 15, 0, 4, 2, 9, 1, 5, 208, 11, 10, 6, 14, 12, 13, 8, 7 }

// convert IRC color codes to ANSI terminal control codes
func convertColorCodes(s string) string {
	out := make([]rune, 0, len(in))
	in := []rune(s)


	bold := false
	color := 0

	for i := 0; i < len(in); i++ {
		c = in[i]
		switch c {
		case '\x02':
			// bold
			bold = true
			out = append(out, emitcolor(bold, color)...)

		case '\x03':
			// color = \x03 number [ ',' number ]
		}

	}
}

func emitcolor(bold bool, color int) []rune {
	s := "\033["
	if bold {
		s += "1;"
	}
	s += "38;5;" + strconv.Itoa(color) + "m"
	return []rune(s)
}
*/

type Handler interface {
	HandleIRC(c *Client, m *Message)
}

type HandlerFunc func(c *Client, m *Message)

func (h HandlerFunc) HandleIRC(c *Client, m *Message) {
	h(c, m)
}

type Channel struct {
	Name  string
	Users []*Hostmask

	namesChan    chan struct{}
	gettingNames bool
}

// Returns a chanel on which names will be sent. Chan receive will block
// until NAMES are ready.
func (c *Channel) Names() <-chan []string {

	ch := make(chan []string)

	go func() {
		if len(c.Users) == 0 {
			// need to send a NAMES command...
		}
		if c.gettingNames {
			<-c.namesChan
		}

		names := make([]string, len(c.Users))
		for i, h := range c.Users {
			names[i] = h.Nick
		}
		ch <- names
	}()

	return ch
}
