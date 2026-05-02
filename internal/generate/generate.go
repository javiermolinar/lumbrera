package generate

import "fmt"

type Files struct {
	Index     string
	Changelog string
	BrainSum  string
}

func FilesForRepo(repo string) (Files, error) {
	_ = repo
	return Files{}, fmt.Errorf("generated files are not implemented yet")
}
