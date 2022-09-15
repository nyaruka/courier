package test

import "os"

func ReadFile(path string) []byte {
	d, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return d
}
