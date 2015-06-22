package core

import (
	"io/ioutil"
	"os"
	"strings"
)

/*
Matcher is a helper object that checks paths against a .tinignore file.
*/
type Matcher struct {
	root      string
	dirRules  []string
	fileRules []string
	empty     bool
}

/*
CreateMatcher creates a new matching object for fast checks against a .tinignore
file. The root path is the directory where the .tinignore file is expected to lie
in.
*/
func CreateMatcher(rootPath string) (*Matcher, error) {
	var matcher Matcher
	matcher.root = rootPath
	allRules, err := readTinIgnore(rootPath)
	if err == ErrNoTinIgnore {
		// if empty we're done
		matcher.empty = true
		return &matcher, nil
	} else if err != nil {
		// return other errors however
		return nil, err
	}
	for _, line := range allRules {
		// is the line a rule for a directory?
		if strings.HasPrefix(line, "/") {
			matcher.dirRules = append(matcher.dirRules, line)
		} else {
			matcher.fileRules = append(matcher.fileRules, line)
		}
	}
	return &matcher, nil
}

/*
Ignore checks whether the given path is to be ignored given the rules within the
root .tinignore file.
*/
func (matcher *Matcher) Ignore(path string) bool {
	// no need to check anything in this case
	if matcher.empty {
		return false
	}
	// start with directories as we always need to check these
	for _, dirLine := range matcher.dirRules {
		// contains because may be subdir already
		if strings.Contains(path, dirLine) {
			return true
		}
	}
	// make sure the path IS a file (no need to check anything otherwise)
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		// check files
		for _, fileLine := range matcher.fileRules {
			// suffix because rest of path doesn't matter for file matches
			if strings.HasSuffix(path, fileLine) {
				return true
			}
		}
	}
	return false
}

/*
IsEmpty can be used to see if the matcher contains any rules at all.
*/
func (matcher *Matcher) IsEmpty() bool {
	return matcher.empty
}

/*
Same returns true if the path is the path for this matcher.
*/
func (matcher *Matcher) Same(path string) bool {
	return path == matcher.root
}

/*
ReadTinIgnore reads the .tinignore file in the given path if it exists. If not
or some other error happens it returns ErrNoTinIgnore.
*/
func readTinIgnore(path string) ([]string, error) {
	data, err := ioutil.ReadFile(path + "/" + TINIGNORE)
	if err != nil {
		// TODO is this correct? Can I be sure that I don't want to know what
		//	    other errors may happen here?
		return nil, ErrNoTinIgnore
	}
	// sanitize (remove empty lines)
	list := strings.Split(string(data), "\n")
	var sanitized []string
	for _, value := range list {
		if value != "" {
			sanitized = append(sanitized, value)
		}
	}
	return sanitized, nil
}
