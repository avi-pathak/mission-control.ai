package tmux

import (
	"reflect"
	"testing"
)

func TestSendKeysArgs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want [][]string
	}{
		{
			"literal text",
			"hello",
			[][]string{{"send-keys", "-t", "s", "-l", "hello"}},
		},
		{
			"approve y then enter",
			"y\n",
			[][]string{
				{"send-keys", "-t", "s", "-l", "y"},
				{"send-keys", "-t", "s", "Enter"},
			},
		},
		{
			"ctrl-c",
			"\x03",
			[][]string{{"send-keys", "-t", "s", "C-c"}},
		},
		{
			"text then enter then text",
			"ab\ncd",
			[][]string{
				{"send-keys", "-t", "s", "-l", "ab"},
				{"send-keys", "-t", "s", "Enter"},
				{"send-keys", "-t", "s", "-l", "cd"},
			},
		},
		{
			"up arrow",
			"\x1b[A",
			[][]string{{"send-keys", "-t", "s", "Up"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SendKeysArgs("s", []byte(c.in))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("SendKeysArgs(%q)\n got=%v\nwant=%v", c.in, got, c.want)
			}
		})
	}
}
