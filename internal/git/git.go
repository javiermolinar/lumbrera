package git

import "fmt"

type Result struct {
	Stdout string
	Stderr string
}

func Run(repo string, args ...string) (Result, error) {
	_ = repo
	_ = args
	return Result{}, fmt.Errorf("git wrapper is not implemented yet")
}
