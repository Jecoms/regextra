package regextra

import (
	"regexp"
)

type Regextra struct {
	*regexp.Regexp
}

// returns value for given regex captureGroupName (subexpression) if found
//  - match -> "{matchedValue}", true
//  - no match -> "", false
func (rex Regextra) SubexpValue(target, cgName string) (string, bool) {
	cgIndex := rex.SubexpIndex(cgName)
	if cgIndex == -1 {
		return "", false
	}

	matches := rex.FindStringSubmatch(target)
	if matches == nil {
		return "", false
	}

	return matches[cgIndex], true
}

// returns map of all regex captureGroupNames (subexpressions) and their values
func (rex Regextra) SubexpMap(target string) map[string]string {
	cgMap := map[string]string{}

	matches := rex.FindStringSubmatch(target)
	if matches == nil {
		return cgMap
	}

	for i, name := range rex.SubexpNames() {
		if i != 0 && name != "" {
			value, ok := rex.SubexpValue(target, name)
			if ok {
				cgMap[name] = value
			}
		}
	}

	return cgMap
}
