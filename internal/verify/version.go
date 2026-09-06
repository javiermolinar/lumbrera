package verify

import (
	"fmt"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

func policiesForVersion(version string) ([]brain.ContentPolicy, error) {
	policies := brain.Policies()
	switch version {
	case brain.Version:
		return policies, nil
	case brain.VersionV2, brain.VersionV1:
		legacy := make([]brain.ContentPolicy, 0, len(policies))
		for _, policy := range policies {
			if policy.Kind == brain.KindNote {
				continue
			}
			if version == brain.VersionV1 && policy.Kind == brain.KindAsset {
				continue
			}
			if policy.Kind == brain.KindWiki {
				policy.EvidenceKinds = []brain.Kind{brain.KindSource}
			}
			legacy = append(legacy, policy)
		}
		return legacy, nil
	default:
		return nil, fmt.Errorf("unsupported Lumbrera brain version %q", version)
	}
}

func managedPoliciesForVersion(version string) ([]brain.ContentPolicy, error) {
	policies, err := policiesForVersion(version)
	if err != nil {
		return nil, err
	}
	managed := make([]brain.ContentPolicy, 0, len(policies))
	for _, policy := range policies {
		if policy.Storage == brain.StorageManagedMarkdown {
			managed = append(managed, policy)
		}
	}
	return managed, nil
}
