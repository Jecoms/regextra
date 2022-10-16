package regextra

import (
	"regexp"
)

type RegexTraPper struct {
	*regexp.Regexp
}

// returns value for given regex captureGroupName (subexpression) if found
//  - match -> "{matchedValue}", true
//  - no match -> "", false
func (rtp RegexTraPper) SubexpValue(target, cgName string) (string, bool) {
	cgIndex := rtp.SubexpIndex(cgName)
	if cgIndex == -1 {
		return "", false
	}

	matches := rtp.FindStringSubmatch(target)
	if matches == nil {
		return "", false
	}

	return matches[cgIndex], true
}

// returns map of all regex captureGroupNames (subexpressions) and their values
func (rtp RegexTraPper) SubexpMap(target string) map[string]string {
	cgMap := map[string]string{}

	matches := rtp.FindStringSubmatch(target)
	if matches == nil {
		return cgMap
	}

	for i, name := range rtp.SubexpNames() {
		if i != 0 && name != "" {
			value, ok := rtp.SubexpValue(target, name)
			if ok {
				cgMap[name] = value
			}
		}
	}

	return cgMap
}
