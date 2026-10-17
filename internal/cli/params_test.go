package cli_test

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/mik3dev/kie-cli/internal/cli"
	"github.com/mik3dev/kie-cli/internal/output"
)

func TestParseParams(t *testing.T) {
	read := func(p string) ([]byte, error) {
		if p == "p.txt" {
			return []byte("from file"), nil
		}
		return nil, os.ErrNotExist
	}
	tests := []struct {
		name  string
		pairs []string
		want  map[string]any
	}{
		{"number", []string{"duration=5"}, map[string]any{"duration": 5.0}},
		{"bool", []string{"sound=true"}, map[string]any{"sound": true}},
		{"string", []string{"prompt=hola mundo"}, map[string]any{"prompt": "hola mundo"}},
		{"object", []string{`x={"a":1}`}, map[string]any{"x": map[string]any{"a": 1.0}}},
		{"array value", []string{`x=[1,2]`}, map[string]any{"x": []any{1.0, 2.0}}},
		{"bad json stays string", []string{`x={nope`}, map[string]any{"x": "{nope"}},
		{"repeat", []string{"image_urls=a", "image_urls=b"}, map[string]any{"image_urls": []any{"a", "b"}}},
		{"repeat x3", []string{"k=a", "k=b", "k=c"}, map[string]any{"k": []any{"a", "b", "c"}}},
		{"file", []string{"prompt=@p.txt"}, map[string]any{"prompt": "from file"}},
		{"escaped at", []string{"k=@@x"}, map[string]any{"k": "@x"}},
		{"first equals only", []string{"k=a=b"}, map[string]any{"k": "a=b"}},
		{"empty value", []string{"k="}, map[string]any{"k": ""}},
		{"numeric-looking text", []string{"k=5 apples"}, map[string]any{"k": "5 apples"}},
		{"padded number stays string", []string{"k= 5"}, map[string]any{"k": " 5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cli.ParseParams(tt.pairs, read)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, err %v; want %#v", got, err, tt.want)
			}
		})
	}
}

func TestParseParamsUsageErrors(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, os.ErrNotExist }
	for _, pairs := range [][]string{{"bad"}, {"=x"}, {"k=@missing.txt"}} {
		_, err := cli.ParseParams(pairs, read)
		var oe *output.Error
		if !errors.As(err, &oe) || oe.Code != output.CodeUsage {
			t.Errorf("%v: err = %v, want usage", pairs, err)
		}
	}
}

func TestMergeInputOverWinsAndDoesNotMutate(t *testing.T) {
	base := map[string]any{"a": 1, "b": 2}
	over := map[string]any{"b": 3, "c": 4}
	got := cli.MergeInput(base, over)
	want := map[string]any{"a": 1, "b": 3, "c": 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if !reflect.DeepEqual(base, map[string]any{"a": 1, "b": 2}) || len(over) != 2 {
		t.Fatalf("inputs mutated: %v %v", base, over)
	}
	if cli.MergeInput(nil, nil) == nil {
		t.Fatal("nil merge must return a non-nil map")
	}
}
