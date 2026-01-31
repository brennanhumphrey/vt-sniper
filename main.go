// Package main implements a CLI tool for monitoring Virginia Tech course sections
// and notifying users when seats become available.
package main

import (
	"log"
)

func main() {
	if err := Run(RunOptions{ConfigPath: "config.json"}); err != nil {
		log.Fatal(err)
	}
}
