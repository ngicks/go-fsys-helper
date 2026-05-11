package osfs

import (
	"os"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Root[*os.File, *Root] = (*Root)(nil)

type Root struct {
	*os.Root
}

func NewRoot(name string) (*Root, error) {
	r, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &Root{Root: r}, nil
}

func (r *Root) IsRoot() {}

func (r *Root) ReadLink(name string) (string, error) {
	return r.ReadLink(name)
}

func (r *Root) OpenRoot(name string) (*Root, error) {
	rr, err := r.Root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &Root{rr}, nil
}
