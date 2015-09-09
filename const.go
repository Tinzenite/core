package core

import "time"

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
