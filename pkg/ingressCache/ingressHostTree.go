package ingressCache

type ingressHostsTree interface {
	set(path string, function string) error // will overwrite existing values if exists
	delete(path string, function string) error
	get(path string) ([]string, error)
}
