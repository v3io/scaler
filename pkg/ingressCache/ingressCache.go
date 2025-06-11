package ingresscache

type IngressHostsTree interface {
	SetFunctionName(path string, function string) error // will overwrite existing values if exists
	DeleteFunctionName(path string, function string) error
	GetFunctionName(path string) ([]string, error)
}
