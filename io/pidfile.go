package io


import (
	"fmt"
	"os"
)


// ----------------------------------------------------------------------------


func CreatePidfile(path string) error {
	return createPidfile(path)
}


// ----------------------------------------------------------------------------


func createPidfile(path string) error {
	var file *os.File
	var err error

	file, err = os.Create(path)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = fmt.Fprintf(file, "%d", os.Getpid())
	if err != nil {
		return err
	}

	return nil
}
