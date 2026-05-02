package manifest

import "fmt"

type Entry struct {
	Path string
	Hash string
}

func Generate(entries []Entry) (string, error) {
	_ = entries
	return "", fmt.Errorf("manifest generation is not implemented yet")
}
