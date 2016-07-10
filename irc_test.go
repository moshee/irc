package irc

import (
	"reflect"
	"testing"
)

type hostmaskTest struct {
	s            string
	expectResult *Hostmask
	expectError  error
}

func TestParseHostmask(t *testing.T) {
	table := []hostmaskTest{
		{"", nil, errHostmaskEmpty},
		{"host", &Hostmask{Address: "host"}, nil},
		{"server.host.tld", &Hostmask{Address: "server.host.tld"}, nil},
		{"nick!malformed", nil, errHostmaskInvalid},
		{"user@malformed", nil, errHostmaskInvalid},
		{"nick@user!malformed", nil, errHostmaskInvalid},
		{"!user@malformed", nil, errHostmaskInvalid},
		{"nick!@malformed", nil, errHostmaskInvalid},
		{"!@", nil, errHostmaskInvalid},
		{"@!", nil, errHostmaskInvalid},
		{"!", nil, errHostmaskInvalid},
		{"@", nil, errHostmaskInvalid},
		{"n!user@wellformed", &Hostmask{"n", "user", "wellformed"}, nil},
		{"nick!u@wellformed", &Hostmask{"nick", "u", "wellformed"}, nil},
		{"*!*@*", &Hostmask{"*", "*", "*"}, nil},
	}

	for _, test := range table {
		h, err := ParseHostmask(test.s)
		if !reflect.DeepEqual(h, test.expectResult) || err != test.expectError {
			t.Errorf("%q: expect %v (%v), got %v (%v)", test.s, test.expectResult, test.expectError, h, err)
		}
	}
}

type messageTest struct {
	s            string
	expectResult *Message
	expectError  error
}

func TestParseMessage(t *testing.T) {
	table := []messageTest{
		{
			"",
			nil,
			errMessageEmpty,
		},
		{
			"QUIT",
			&Message{Command: "QUIT"},
			nil,
		},
		{
			"PING 12345",
			&Message{Command: "PING", Params: []string{"12345"}},
			nil,
		},
		{
			"PING :a.b.c",
			&Message{Command: "PING", Trailing: "a.b.c"},
			nil,
		},
		{
			":moshee!~moshee@mo.sh.ee PRIVMSG #roboworld :hi",
			&Message{From: &Hostmask{"moshee", "~moshee", "mo.sh.ee"}, Command: "PRIVMSG", Params: []string{"#roboworld"}, Trailing: "hi"},
			nil,
		},
		{
			":a.b.c CMD",
			&Message{From: &Hostmask{Address: "a.b.c"}, Command: "CMD"},
			nil,
		},
		{
			":a.b.c CMD param1",
			&Message{From: &Hostmask{Address: "a.b.c"}, Command: "CMD", Params: []string{"param1"}},
			nil,
		},
		{
			"CMD param1 param2",
			&Message{Command: "CMD", Params: []string{"param1", "param2"}},
			nil,
		},
		{
			":a.b.c CMD param1 param2 :trailing",
			&Message{From: &Hostmask{Address: "a.b.c"}, Command: "CMD", Params: []string{"param1", "param2"}, Trailing: "trailing"},
			nil,
		},
		{
			":a.b.c CMD param:1 p:aram2 :trailing trailing",
			&Message{From: &Hostmask{Address: "a.b.c"}, Command: "CMD", Params: []string{"param:1", "p:aram2"}, Trailing: "trailing trailing"},
			nil,
		},
		{
			":a.b.c CMD p1 p2 p3 p4 p5 p6 p7 p8 p9 p10 p11 p12 p13 p14 trailing trailing trailing",
			&Message{
				From:    &Hostmask{Address: "a.b.c"},
				Command: "CMD",
				Params: []string{
					"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "p10", "p11", "p12", "p13", "p14",
				},
				Trailing: "trailing trailing trailing",
			},
			nil,
		},
		{
			":a.b.c CMD :trailing trailing",
			&Message{From: &Hostmask{Address: "a.b.c"}, Command: "CMD", Trailing: "trailing trailing"},
			nil,
		},
		{
			":a.b.c",
			nil,
			errMessageInvalid,
		},
	}

	for _, test := range table {
		m, err := ParseMessage(test.s)
		if !reflect.DeepEqual(m, test.expectResult) || err != test.expectError {
			t.Errorf("%q: expect %#v (%v), got %#v (%v)", test.s, test.expectResult, test.expectError, m, err)
		}
	}
}
