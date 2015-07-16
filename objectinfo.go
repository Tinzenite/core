package core

import "encoding/json"

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
	stin, err := createStaticInfo(path.FullPath(), selfid)
	if err != nil {
		return nil, err
	}
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
String returns a json representation of this object.
*/
func (o *ObjectInfo) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
