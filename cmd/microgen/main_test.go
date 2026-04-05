package main

import (
	"reflect"
	"testing"
)

func TestSplitComma(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{",,a,,b,,", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitComma(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitComma(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		cfg  config
		pass bool
	}{
		{config{fromDB: false, idlPath: ""}, false},
		{config{fromDB: true, idlPath: ""}, true},
		{config{fromDB: false, idlPath: "test.go"}, true},
	}
	for i, c := range cases {
		err := c.cfg.validate()
		if (err == nil) != c.pass {
			t.Errorf("case %d: validate() error = %v, want pass = %v", i, err, c.pass)
		}
	}
}
