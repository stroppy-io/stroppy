//go:build windows

package toolchain

import (
	"io"
	"os"

	"golang.org/x/term"
)

func interactive(input io.Reader, output io.Writer) bool {
	inputFile, inputOK := input.(*os.File)
	outputFile, outputOK := output.(*os.File)

	return inputOK && outputOK && term.IsTerminal(int(inputFile.Fd())) && term.IsTerminal(int(outputFile.Fd()))
}
