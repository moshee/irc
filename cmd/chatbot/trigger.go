package main

import "ktkr.us/pkg/irc"

var triggers = map[string]irc.Handler{}

func handleTrigger(c *irc.Client, m *irc.Message) bool {
	return false
}
