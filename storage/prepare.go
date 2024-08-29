package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"

	"github.com/spf13/afero"
)

var ErrInvalidInput = errors.New("invalid input")

var (
	aferoFsType = reflect.TypeOf((*afero.Fs)(nil)).Elem()
	fsFsType    = reflect.TypeOf((*fs.FS)(nil)).Elem()
)

func PrepareHandle[S, H any](base afero.Fs, pathSet S, initialContents any) (H, error) {
	var handle, zero H

	sRv := reflect.ValueOf(pathSet)
	hRv := reflect.ValueOf(&handle)

	if err := validPrepareInput(sRv, hRv); err != nil {
		return zero, err
	}

	hRv = hRv.Elem()

	for i := 0; i < sRv.NumField(); i++ {
		field := sRv.Field(i)
		// field.String() does not panic upon invoked for non string field.
		// That's not what we want it to be.
		name, path := sRv.Type().Field(i).Name, field.Interface().(string)
		path = filepath.Clean(filepath.FromSlash(path))

		err := base.MkdirAll(path, fs.ModeDir|0o777)
		if err != nil {
			return zero, err
		}

		fsys := afero.NewBasePathFs(base, path)
		aferoField := hRv.FieldByName(name)
		aferoField.Set(reflect.ValueOf(fsys))
	}

	if initialContents != nil {
		icRv := reflect.ValueOf(initialContents)
		err := validCopyContentsInput(hRv, icRv, true)
		if err != nil {
			return zero, err
		}
		err = CopyContents(handle, initialContents)
		if err != nil {
			return zero, err
		}
	}

	return handle, nil

}

// func isEmpty(s string) bool {
// 	// filepath.Clean converts "" to "."
// 	return s == "" || s == "."
// }

// func newNotLocalErr(name, path string) error {
// 	return fmt.Errorf("%w: %s is not a local path, path = %s", ErrInvalidInput, name, path)
// }

func ValidatePrepareInput(pathSet, pathHandle any) error {
	return validPrepareInput(reflect.ValueOf(pathSet), reflect.ValueOf(pathHandle))
}

func validPrepareInput(sRv, hRv reflect.Value) error {
	if sRv.Kind() == reflect.Invalid && hRv.Kind() == reflect.Invalid {
		// both nil indicates no path and no handle.
		return nil
	}

	if sRv.Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: dirSet must be a struct type but kind is %s",
			ErrInvalidInput, sRv.Kind(),
		)
	}

	if hRv.Kind() != reflect.Pointer || hRv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: pathHandle must be a pointer type pointing to a struct but kind is %s",
			ErrInvalidInput, hRv.Kind(),
		)
	}

	hRv = hRv.Elem()

	if sRv.NumField() != hRv.NumField() {
		return fmt.Errorf(
			"%w: unmatched NumField, dirSet and pathHandle must have exact same keyed exported fields,"+
				" dirSet has %d fields, pathHandle has %d fields.",
			ErrInvalidInput, sRv.NumField(), hRv.NumField(),
		)
	}

	hFieldSet := make(map[string]struct{})
	for i := 0; i < hRv.NumField(); i++ {
		hFieldSet[hRv.Type().Field(i).Name] = struct{}{}
		field := hRv.Type().Field(i)
		if !field.Type.Implements(aferoFsType) {
			return fmt.Errorf(
				"%w: pathHandle must only have exported afero.Fs field, but is %s",
				ErrInvalidInput, field.Type.String(),
			)
		}
	}

	for i := 0; i < sRv.NumField(); i++ {
		// It does not need to be exact same layout (definition order).
		dirSetField := sRv.Field(i)
		dirSetFieldName := sRv.Type().Field(i).Name
		if dirSetField.Kind() != reflect.String {
			return fmt.Errorf(
				"%w: dirSet must only have exported string fields, but field %s has %s field",
				ErrInvalidInput, dirSetFieldName, dirSetField.Kind(),
			)
		}
		if _, ok := hFieldSet[dirSetFieldName]; !ok {
			return fmt.Errorf(
				"%w: dirSet and pathHandle must have exact same keyed exported fields, but field %s does not exist in pathHandle",
				ErrInvalidInput, dirSetFieldName,
			)
		}

		v := dirSetField.Interface().(string)
		if v == "" {
			return fmt.Errorf("%w: dirSet specifies empty directory", ErrInvalidInput)
		}
		if !filepath.IsLocal(v) {
			return fmt.Errorf("%w: dirSet specifies absolute directory or parent directory.", ErrInvalidInput)
		}
	}

	return nil
}
