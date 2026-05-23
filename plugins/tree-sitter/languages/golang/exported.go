package golang

func IsExported(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}
