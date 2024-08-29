// example is example package for gentypes.
package example

//go:generate go run ../cmd/gentypes -pkg example -name Path -fields Foo,Bar,Baz -o ./example.generated.go
