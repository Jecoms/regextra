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
