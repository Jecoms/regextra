package regextra

import (
	"reflect"
	"regexp"
	"testing"
)

var (
	testPattern = regexp.MustCompile(`(?P<first>one) (?P<second>two) (?P<second>again) three`)
	testTarget  = "one two again three"
)

func TestRegexTraPper_SubexpValue(t *testing.T) {
	type fields struct {
		Regexp *regexp.Regexp
	}
	type args struct {
		target     string
		subexpName string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      string
		wantFound bool
	}{
		{
			name: "found one",
			fields: fields{
				Regexp: testPattern,
			},
			args: args{
				target:     testTarget,
				subexpName: "first",
			},
			want:      "one",
			wantFound: true,
		},
		{
			name: "found two",
			fields: fields{
				Regexp: testPattern,
			},
			args: args{
				target:     testTarget,
				subexpName: "second",
			},
			want:      "two",
			wantFound: true,
		},
		{
			name: "find none",
			fields: fields{
				Regexp: testPattern,
			},
			args: args{
				target:     testTarget,
				subexpName: "third",
			},
			want:      "",
			wantFound: false,
		},
		{
			name: "get the price",
			fields: fields{
				Regexp: regexp.MustCompile(`(?P<price>\$\d+(,\d{3})*(\.\d{1,2})?)`),
			},
			args: args{
				target:     "The price is $1,234.56",
				subexpName: "price",
			},
			want:      "$1,234.56",
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtp := RegexTraPper{
				Regexp: tt.fields.Regexp,
			}
			got, found := rtp.SubexpValue(tt.args.target, tt.args.subexpName)
			if got != tt.want {
				t.Errorf("RegexTraPper.SubexpValue() got = %v, want %v", got, tt.want)
			}
			if found != tt.wantFound {
				t.Errorf("RegexTraPper.SubexpValue() found = %v, wantFound %v", found, tt.wantFound)
			}
		})
	}
}

func TestRegexTraPper_SubexpMap(t *testing.T) {
	type fields struct {
		Regexp *regexp.Regexp
	}
	type args struct {
		target string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]string
	}{
		{
			name: "found group names with values",
			fields: fields{
				Regexp: testPattern,
			},
			args: args{
				target: testTarget,
			},
			want: map[string]string{
				"first":  "one",
				"second": "two",
			},
		},
		{
			name: "found none",
			fields: fields{
				Regexp: testPattern,
			},
			args: args{
				target: "one two three",
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtp := RegexTraPper{
				Regexp: tt.fields.Regexp,
			}
			if got := rtp.SubexpMap(tt.args.target); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RegexTraPper.SubexpMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
