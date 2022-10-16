package regextra

import (
	"reflect"
	"regexp"
	"testing"
)

func TestSubexpMap(t *testing.T) {
	type args struct {
		re     *regexp.Regexp
		target string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "found group names with values",
			args: args{
				re:     testPattern,
				target: testTarget,
			},
			want: map[string]string{
				"first":  "one",
				"second": "two",
			},
		},
		{
			name: "found none",
			args: args{
				re:     testPattern,
				target: "one two three",
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubexpMap(tt.args.re, tt.args.target); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SubexpMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubexpValue(t *testing.T) {
	type args struct {
		re     *regexp.Regexp
		target string
		cgName string
	}
	tests := []struct {
		name      string
		args      args
		want      string
		wantFound bool
	}{
		{
			name: "found one",
			args: args{
				re:     testPattern,
				target: testTarget,
				cgName: "first",
			},
			want:      "one",
			wantFound: true,
		},
		{
			name: "found two",
			args: args{
				re:     testPattern,
				target: testTarget,
				cgName: "second",
			},
			want:      "two",
			wantFound: true,
		},
		{
			name: "find none",
			args: args{
				re:     testPattern,
				target: testTarget,
				cgName: "third",
			},
			want:      "",
			wantFound: false,
		},
		{
			name: "get the price",
			args: args{
				re:     regexp.MustCompile(`(?P<price>\$\d+(,\d{3})*(\.\d{1,2})?)`),
				target: "The price is $1,234.56",
				cgName: "price",
			},
			want:      "$1,234.56",
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := SubexpValue(tt.args.re, tt.args.target, tt.args.cgName)
			if got != tt.want {
				t.Errorf("SubexpValue() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.wantFound {
				t.Errorf("SubexpValue() got1 = %v, want %v", got1, tt.wantFound)
			}
		})
	}
}
