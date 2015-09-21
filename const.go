package core

import (
	"errors"
	"time"
)

/*
transferTimeout is the time after which a file is re-requested.
*/
const transferTimeout = 1 * time.Minute

/*
Naming of conflicting files.

TODO: this should be improved because it can quickly cause multi merge
problems... Consider using name of peers and version numbers.
*/
const (
	LOCAL  = ".LOCAL"
	REMOTE = ".REMOTE"
	MODEL  = ".MODEL"
)

var (
	errAuthEncryption      = errors.New("encryption failed")
	errAuthDecryption      = errors.New("decryption failed")
	errAuthInvalidKeys     = errors.New("keys are invalid")
	errAuthInvalidSecure   = errors.New("secure is invalid")
	errAuthInvalidPassword = errors.New("password derived keys are incorrect")
	errPeerUnknown         = errors.New("peer is unknown")
	errPeerUnauthenticated = errors.New("peer is unauthenticated")
)
