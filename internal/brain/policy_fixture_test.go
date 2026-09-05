package brain

import "testing"

// InstallPoliciesForTest replaces the closed policy table for a black-box test.
// It is compiled only into the brain package's test binary.
func InstallPoliciesForTest(t testing.TB, policies []ContentPolicy) {
	t.Helper()
	previous := clonePolicySlice(contentPolicies)
	contentPolicies = clonePolicySlice(policies)
	t.Cleanup(func() {
		contentPolicies = previous
	})
}
