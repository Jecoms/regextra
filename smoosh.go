package regextra

import (
	"regexp"
)

func SubexpValue(re *regexp.Regexp, target, cgName string) (string, bool) {
	return Regextra{re}.SubexpValue(target, cgName)
}

func SubexpMap(re *regexp.Regexp, target string) map[string]string {
	return Regextra{re}.SubexpMap(target)
}
