package core

/*
Objectinfo represents the in model object fully.
*/
type objectInfo struct {
	directory      bool // safety check wether the obj is a dir
	Identification string
	Name           string
	Path           string
	Shadow         bool
	Version        map[string]int
	Objects        []*objectInfo `json:",omitempty"`
	Content        string        `json:",omitempty"`
}

/*
Equal checks wether the given pointer points to the same object based on pointer
and identification. NOTE: Does not compare any other properties!
*/
func (o *objectInfo) equal(that *objectInfo) bool {
	return o == that || o.Identification == that.Identification
}

func (o *objectInfo) apply(that *objectInfo) error {
	if !o.equal(that) {
		return ErrWrongObject
	}
	return nil
}
