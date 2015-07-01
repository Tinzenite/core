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
	Directory      bool
	Content        string
	Modtime        time.Time
	Version        version
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

/*
UpdateFromDisk updates the hash and modtime to match the file on disk.
*/
func (s *staticinfo) UpdateFromDisk(path string) error {
	if !s.Directory {
		hash, err := contentHash(path)
		if err != nil {
			return err
		}
		s.Content = hash
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	s.Modtime = stat.ModTime()
	return nil
}
