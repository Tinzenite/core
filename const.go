package core

import "github.com/tinzenite/shared"

/*
Naming of conflicting files.

TODO: this should be improved because it can quickly cause multi merge
problems... Consider using name of peers and version numbers.
*/
const (
	LOCAL  = ".LOCAL"
	REMOTE = ".REMOTE"
)

// .tinignore content for .tinzenite directory
const TINDIRIGNORE = "# DO NOT MODIFY!\n/" + shared.LOCALDIR + "\n/" + shared.TEMPDIR + "\n/" + shared.RECEIVINGDIR
