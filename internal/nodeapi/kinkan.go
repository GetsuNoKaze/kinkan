package nodeapi

// Kinkan's fields of the Node API. Each is embedded first in Mikan's struct: its JSON
// keys stay where they were (an embedded struct's fields are the outer one's), and
// upstream's new fields, added at the end, do not meet the fork's line in a merge.
// Mikan's nodes and panels ignore them.

// KinkanState is in DesiredState.
type KinkanState struct {
	// Site is the website the node shows (kinkan_site.go); nil: none.
	Site *SiteState `json:"site,omitempty"`
}

// KinkanValidate is in ValidateRequest.
type KinkanValidate struct {
	// SitePort is the node's website over TLS, a REALITY target the node may use.
	SitePort int `json:"site_port,omitempty"`
}

// KinkanHealth is in Health.
type KinkanHealth struct {
	// Site is the website the node shows; nil when none (or a node without sites).
	Site *SiteStatus `json:"site,omitempty"`
}

// KinkanApply is in ApplyResult.
type KinkanApply struct {
	// SiteMissing: the node does not have the state's site; the panel sends it (PutSite)
	// and applies again.
	SiteMissing bool `json:"site_missing,omitempty"`
}
