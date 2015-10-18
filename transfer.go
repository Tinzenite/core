package core

import "time"

/*
transfer is a structure for keeping track of active in transfers.
*/
type transfer struct {
	updated time.Time // last time this transfer was updated for timeout reasons
	active  string    // active stores the address of the peer from which we're fetching the file
	done    onDone    // function to execute once the file has been received
}

/*
onDone is called when the transfer is successfully completed.
*/
type onDone func(address, path string)
