package core

import (
	"encoding/json"
	"io/ioutil"
)

/*
Authentication file.
*/
type Authentication struct {
	User    string
	Dirname string
	DirID   string
	Key     string
}

/*
loadAuthentication loads the auth.json file for the given Tinzenite directory.
*/
func loadAuthentication(root string) (*Authentication, error) {
	data, err := ioutil.ReadFile(root + "/" + TINZENITEDIR + "/" + ORGDIR + "/" + AUTHJSON)
	if err != nil {
		return nil, err
	}
	auth := &Authentication{}
	err = json.Unmarshal(data, auth)
	if err != nil {
		return nil, err
	}
	return auth, nil
}

func (a *Authentication) store(root string) error {
	// write auth file
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(root+"/"+TINZENITEDIR+"/"+ORGDIR+"/"+AUTHJSON, data, FILEPERMISSIONMODE)
}
