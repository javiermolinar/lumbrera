package hooks

import "fmt"

func Install(repo string) error {
	_ = repo
	return fmt.Errorf("hook installation is not implemented yet")
}
