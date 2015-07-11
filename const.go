package core

import "errors"

/*
Errors of Tinzenite.
*/
var (
	ErrUnsupported      = errors.New("Feature currently unsupported!")
	ErrIsTinzenite      = errors.New("Already a Tinzenite directory!")
	ErrNotTinzenite     = errors.New("Path is not valid Tinzenite directory!")
	ErrNoTinIgnore      = errors.New("No .tinignore file found!")
	ErrUntracked        = errors.New("Object is not tracked in the model!")
	ErrNilInternalState = errors.New("Internal state has illegal NIL values!")
)

/*
Internal errors of Tinzenite.
*/
var (
	errWrongObject      = errors.New("Wrong ObjectInfo!")
	errConflict         = errors.New("Conflict, can not apply!")
	errIllegalFileState = errors.New("Illegal file state detected!")
	errModelInconsitent = errors.New("Model tracked and staticinfo are inconsistent!")
)

// constant value here
const (
	/*RANDOMSEEDLENGTH is the amount of bytes used as cryptographic hash seed.*/
	RANDOMSEEDLENGTH = 32
	/*IDMAXLENGTH is the length in chars of new random identification hashes.*/
	IDMAXLENGTH = 16
	/*KEYLENGTH is the length of the encryption key used for challenges and file encryption.*/
	KEYLENGTH = 256
	/*FILEPERMISSIONMODE used for all file operations.*/
	FILEPERMISSIONMODE = 0777
	/*CHUNKSIZE for hashing and encryption.*/
	CHUNKSIZE = 8 * 1024
)

// Path constants here
const (
	TINZENITEDIR  = ".tinzenite"
	TINIGNORE     = ".tinignore"
	DIRECTORYLIST = "directory.list"
	LOCAL         = "local"
	TEMP          = "temp"
	ORGDIR        = "org"
	PEERSDIR      = "peers"
	ENDING        = ".json"
	AUTHJSON      = "auth" + ENDING
	MODELJSON     = "model" + ENDING
	SELFPEERJSON  = "self" + ENDING
)

// .tinignore content for .tinzenite directory
const TINDIRIGNORE = "# DO NOT MODIFY!\n/" + LOCAL + "\n/" + TEMP
