package storage

import (
	"fmt"
	"io/fs"
	"reflect"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/spf13/afero"
)

// CopyContents copies each field of contents to its corresponding field of pathHandle.
//
// pathHandle and contents must be flat structs and
// must only contain exported afero.Fs, fs.FS fields respectively.
//
//	type pathHandle struct {
//		RuntimeEnvFiles afero.Fs
//	}
//
//	type Contents struct {
//		RuntimeEnvFiles fs.FS
//	}
//
//	composePath, err := composeloader.CopyContents(
//		pathHandle,
//		Contents{
//			RuntimeEnvFiles: fstest.MapFS{
//				"foo.env": &fstest.MapFile{
//					Data:    []byte{},
//					Mode:    0o664,
//					ModTime: time.Now(),
//				},
//				"bar.env": &fstest.MapFile{
//					Data:    []byte{},
//					Mode:    0o664,
//					ModTime: time.Now(),
//				},
//			},
//		},
//	)
func CopyContents(pathHandle, contents any) error {
	hRv := reflect.ValueOf(pathHandle)
	cRv := reflect.ValueOf(contents)

	if err := validCopyContentsInput(hRv, cRv, false); err != nil {
		return err
	}

	if hRv.Kind() == reflect.Pointer && !hRv.IsNil() {
		hRv = hRv.Elem()
	}
	if cRv.Kind() == reflect.Pointer && !cRv.IsNil() {
		cRv = cRv.Elem()
	}

	for i := 0; i < hRv.NumField(); i++ {
		hf := hRv.Field(i)
		cf := cRv.Field(i)

		if cf.IsNil() {
			continue
		}

		if err := fsutil.CopyFS(hf.Interface().(afero.Fs), cf.Interface().(fs.FS)); err != nil {
			return err
		}
	}

	return nil
}

func ValidateCopyContentsInput(pathHandle, dirContents any, allowNilField bool) error {
	return validCopyContentsInput(reflect.ValueOf(pathHandle), reflect.ValueOf(dirContents), allowNilField)
}

func validCopyContentsInput(hRv, cRv reflect.Value, allowNilField bool) error {
	if hRv.Kind() == reflect.Pointer && !hRv.IsNil() {
		hRv = hRv.Elem()
	}
	if cRv.Kind() == reflect.Pointer && !cRv.IsNil() {
		cRv = cRv.Elem()
	}

	if hRv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: pathHandle is not a struct", ErrInvalidInput)
	}
	if cRv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: initialContents is not a struct", ErrInvalidInput)
	}

	if hRv.NumField() != cRv.NumField() {
		return fmt.Errorf("%w: pathHandle and initialContents mismatches their NumField", ErrInvalidInput)
	}

	fieldNames := map[string]struct{}{}
	for i := 0; i < hRv.NumField(); i++ {
		st := hRv.Type().Field(i)

		fieldNames[st.Name] = struct{}{}

		if !st.Type.Implements(aferoFsType) {
			return fmt.Errorf(
				"%w: pathHandle must only have exported afero.Fs field, but is %s",
				ErrInvalidInput, st.Type.String(),
			)
		}
		if !allowNilField && hRv.Field(i).IsNil() {
			return fmt.Errorf("%w: pathHandle must not have nil field", ErrInvalidInput)
		}
	}

	for i := 0; i < cRv.NumField(); i++ {
		st := cRv.Type().Field(i)

		if !st.Type.Implements(fsFsType) {
			return fmt.Errorf(
				"%w: contents must only have exported fs.FS field, but is %s",
				ErrInvalidInput, st.Type.String(),
			)
		}

		if _, ok := fieldNames[st.Name]; !ok {
			return fmt.Errorf(
				"%w: pathHandle and contents must have exact same keyed exported fields, but field %s does not exist in pathHandle",
				ErrInvalidInput, st.Name,
			)
		}
	}

	return nil
}
