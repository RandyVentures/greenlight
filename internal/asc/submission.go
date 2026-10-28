package asc

import (
	"fmt"
	"net/url"
)

// ForSubmission returns an isolated client pinned to exactly one app version.
// Without an explicit ID, only a single editable version is safe to select.
func (c *Client) ForSubmission(appID, versionID, platform string) (*Client, *AppStoreVersion, error) {
	versions, err := c.GetAppStoreVersions(appID)
	if err != nil {
		return nil, nil, err
	}
	var candidates []AppStoreVersion
	for _, v := range versions {
		if platform != "" && v.Attributes.Platform != platform {
			continue
		}
		if versionID != "" {
			if v.ID == versionID {
				candidates = append(candidates, v)
			}
			continue
		}
		switch v.Attributes.AppStoreState {
		case "PREPARE_FOR_SUBMISSION", "DEVELOPER_REJECTED", "REJECTED", "METADATA_REJECTED":
			candidates = append(candidates, v)
		}
	}
	if len(candidates) != 1 {
		return nil, nil, fmt.Errorf("found %d matching submission versions; specify --version-id and, if needed, --platform", len(candidates))
	}
	scoped := *c
	scoped.scopedApp = appID
	scoped.scopedVersion = &candidates[0]
	scoped.scopedBuild = nil
	scoped.scopedBuildFetched = false
	return &scoped, scoped.scopedVersion, nil
}

// GetVersionBuild follows the selected version's build relationship. A nil
// result means no build is attached; uploaded builds are never a substitute.
func (c *Client) GetVersionBuild(versionID string) (*Build, error) {
	if c.scopedVersion != nil && c.scopedVersion.ID == versionID && c.scopedBuildFetched {
		return c.scopedBuild, nil
	}
	var resp DataResponse[*Build]
	if err := c.get("/appStoreVersions/"+url.PathEscape(versionID)+"/build", &resp); err != nil {
		return nil, err
	}
	if c.scopedVersion != nil && c.scopedVersion.ID == versionID {
		c.scopedBuild = resp.Data
		c.scopedBuildFetched = true
	}
	return resp.Data, nil
}
