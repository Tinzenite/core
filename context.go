package core

import "errors"

// Errors for Context
var (
	ErrContextNameEmpty  = errors.New("Name is empty!")
	ErrContextData       = errors.New("ToxData error!")
	ErrContextDataExists = errors.New("ToxData already exists!")
)

// Context is the peer context that Core will work with.
type Context struct {
	// Name of the peer.
	Name string
	// Address in hex form.
	Address string
	// The Tox save data.
	ToxData []byte
}
