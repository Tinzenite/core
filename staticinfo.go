package core

import (
	"os"
	"time"
)

/*
staticinfo stores all information that Tinzenite must keep between calls to
m.Update(). This includes the object ID and version for reapplication, plus
the content hash if required for file content changes detection.
*/
type staticinfo struct {
	Identification string
	Version        map[string]int
	Directory      bool
	Content        string
	Modtime        time.Time
}

/*
createStaticInfo for the given file at the path with all values filled
accordingly.
*/
func createStaticInfo(path, selfpeerid string) (*staticinfo, error) {
	// fetch all values we'll need to store
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	hash := ""
	if !stat.IsDir() {
		hash, err = contentHash(path)
		if err != nil {
			return nil, err
		}
	}
	return &staticinfo{
		Identification: id,
		Version:        map[string]int{selfpeerid: 0}, // set initial version
		Directory:      stat.IsDir(),
		Content:        hash,
		Modtime:        stat.ModTime()}, nil
}
