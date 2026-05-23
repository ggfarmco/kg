package types

type Repo struct {
	Name string
}

func (r *Repo) Save() error { return nil }

type Reader interface {
	Read() error
}
