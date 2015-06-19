package core

import "errors"

/*
Errors of Tinzenite.
*/
var (
	ErrUnsupported  = errors.New("Feature currently unsupported!")
	ErrIsTinzenite  = errors.New("Already a Tinzenite directory!")
	ErrNotTinzenite = errors.New("Path is not valid Tinzenite directory!")
	ErrNoTinIgnore  = errors.New("No .tinignore file found!")
)

// constant value here
const (
	// RANDOMSEEDLENGTH is the amount of bytes used as cryptographic hash seed.
	RANDOMSEEDLENGTH = 32
	// IDMAXLENGTH is the length in chars of new random identification hashes.
	IDMAXLENGTH        = 16
	FILEPERMISSIONMODE = 0777
)

// Path constants here
const (
	TINZENITEDIR  = ".tinzenite"
	TINIGNORE     = ".tinignore"
	DIRECTORYLIST = "directory.list"
)

/*
CommunicationMethod is an enumeration for the available communication methods
of Tinzenite peers.
*/
type CommunicationMethod int

const (
	none CommunicationMethod = iota
	tox
)

/*
type Object interface {
}

type Directory struct {
	_    Object
	Name string
}

type File struct {
	_    Object
	Name string
}
*/