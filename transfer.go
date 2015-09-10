package core

import "time"

/*
transfer is a structure for keeping track of active in transfers.
*/
type transfer struct {
	// last time this transfer was updated for timeout reasons
	updated time.Time
	// peers stores the addresses of all known peers that have the file update
	peers []string
	// function to execute once the file has been received
	done onDone
}

/*
onDone is called when the transfer is successfully completed.
*/
type onDone func(address, path string)
