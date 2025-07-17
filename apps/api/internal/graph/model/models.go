package model

type Project struct {
	ID             string `json:"id"`
	K8sName        string `json:"-"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	OrganisationID int32  `json:"-"`
}

type Deployment struct {
	ID             string `json:"id"`
	PreviewURL     string `json:"previewUrl"`
	ProjectID      string `json:"-"`
	OrganisationID int32  `json:"-"`
}
