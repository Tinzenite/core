package core

/*TODO: make everything private*/

/*Model todo*/
type Model struct {
	pathexisted map[string]bool
}

/*Objectinfo todo*/
type Objectinfo struct {
	directory      bool
	Identification string
	Name           string
	Path           string
	Shadow         bool
	Version        map[string]int
	// Objects        []*Objectinfo `json:",omitempty"`
	Content string `json:",omitempty"`
}

/*LoadModel todo*/
func LoadModel(path string) (*Model, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	m := &Model{}

	return m, nil
}

/*Update todo*/
func (m *Model) Update() (bool, error) {
	/*TODO*/
	return false, ErrUnsupported
}

/*Register todo*/
func (m *Model) Register(v chan Objectinfo) {
	/*TODO*/
}

/*Read todo*/
func (m *Model) Read() (*Objectinfo, error) {
	/*TODO*/
	return nil, ErrUnsupported
}
