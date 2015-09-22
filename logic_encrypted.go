package core

import "github.com/tinzenite/shared"

/*
onEncryptedMessage is called for messages from encrypted peers. Will redestribute
the message according to its type.
*/
func (c *chaninterface) onEncryptedMessage(address string, msgType shared.MsgType, message string) {
	switch msgType {
	// TODO switch and handle messages
	default:
		c.warn("Unknown object received:", msgType.String())
	}
}
