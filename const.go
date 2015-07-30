package core

import "time"

/*
transferTimeout is the time where a file transfer is accepted after a request.
*/
const transferTimeout = 5 * time.Second

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
