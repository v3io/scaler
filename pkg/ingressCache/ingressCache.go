package ingressCache

type IngressHostsTree interface {
	SetPath(path string, function string) error // will overwrite existing values if exists
	DeletePath(path string, function string) error
	GetPath(path string) ([]string, error)
}
