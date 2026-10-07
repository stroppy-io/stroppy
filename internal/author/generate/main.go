// Command generate refreshes explicitly published builtin source support files.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	directoryMode = 0o755
	fileMode      = 0o644
)

//nolint:gocognit,cyclop,gosec,nestif // copies public source and licenses, never credential-bearing files.
func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(filepath.Join(root, "internal", "author", "LICENSE"), license, fileMode); err != nil {
		panic(err)
	}

	helper, err := os.ReadFile(filepath.Join(root, "workloads", "internal", "workloadtest", "contract.go"))
	if err != nil {
		panic(err)
	}

	for _, name := range []string{"simple", "baseline", "execute_sql", "tpcb", "tpcc", "tpch", "tpcds"} {
		directory := filepath.Join(root, "workloads", name)
		if err := os.WriteFile(filepath.Join(directory, "LICENSE"), license, fileMode); err != nil {
			panic(err)
		}

		if name == "tpcb" || name == "tpcc" || name == "tpch" || name == "tpcds" {
			target := filepath.Join(directory, "published", "workloadtest")
			if err := os.MkdirAll(target, directoryMode); err != nil {
				panic(err)
			}

			if err := os.WriteFile(filepath.Join(target, "contract.go"), helper, fileMode); err != nil {
				panic(err)
			}
		}

		if name == "tpch" || name == "tpcds" {
			input := filepath.Join(root, "pkg", "datagen", "source")

			entries, err := os.ReadDir(input)
			if err != nil {
				panic(err)
			}

			target := filepath.Join(directory, "published", "source")
			if err := os.MkdirAll(target, directoryMode); err != nil {
				panic(err)
			}

			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}

				data, err := os.ReadFile(filepath.Join(input, entry.Name()))
				if err != nil {
					panic(err)
				}

				if err := os.WriteFile(filepath.Join(target, entry.Name()), data, fileMode); err != nil {
					panic(err)
				}
			}
		}

		fmt.Fprintln(os.Stdout, name)
	}
}
