package tests

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/nsf/jsondiff"
	"github.com/rekki/reveal/reveal"
)

func TestReveal(t *testing.T) {
	_, currentFilename, _, _ := runtime.Caller(0)
	currentDirname := path.Dir(currentFilename)

	files, err := os.ReadDir(currentDirname)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		dirname := file.Name()

		t.Run(dirname, func(t *testing.T) {
			t.Parallel()

			expected, err := ioutil.ReadFile(path.Join(dirname, "openapi3.json"))
			if err != nil {
				panic(err)
			}

			out, err := reveal.Reveal(context.Background(), dirname)
			if err != nil {
				panic(err)
			}

			outjson, err := json.Marshal(out)
			if err != nil {
				panic(err)
			}

			opts := jsondiff.DefaultConsoleOptions()
			diff, msg := jsondiff.Compare(expected, outjson, &opts)
			if diff != jsondiff.FullMatch {
				t.Error(msg)
			}
		})
	}
}
