package core

import "log"

/*
ObjectInfo represents the in model object fully.
*/
type ObjectInfo struct {
	directory      bool // safety check wether the obj is a dir
	Identification string
	Name           string
	Path           string
	Shadow         bool
	Version        version
	Content        string        `json:",omitempty"`
	Objects        []*ObjectInfo `json:",omitempty"`
}

/*
createObjectInfo is a TEST function for creating an object for the specified
parameters.
*/
func createObjectInfo(root string, subpath string, selfid string) (*ObjectInfo, error) {
	path := createPath(root, subpath)
	stin, _ := createStaticInfo(path.FullPath(), selfid)
	return &ObjectInfo{
		directory:      stin.Directory,
		Identification: stin.Identification,
		Name:           path.LastElement(),
		Path:           path.Subpath(),
		Shadow:         false,
		Version:        stin.Version,
		Content:        stin.Content}, nil
}

/*
Equal checks wether the given pointer points to the same object based on pointer
and identification. NOTE: Does not compare any other properties!
*/
func (o *ObjectInfo) Equal(that *ObjectInfo) bool {
	return o == that || o.Identification == that.Identification
}

/*

*/
func (o *ObjectInfo) apply(that *ObjectInfo, selfpeerid string) error {
	// if not same object break right away
	if !o.Equal(that) {
		return errWrongObject
	}
	// first check for sanity
	if o.Version[selfpeerid] != that.Version[selfpeerid] {
		log.Println("Something BIG went wrong!")
		return errConflict
	}
	/*TODO need to put more thought into this...*/
	return ErrUnsupported
}
