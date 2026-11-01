// Command schemagen writes the generated JSON Schemas into a directory,
// replacing what was there (run through go generate in contracts/events/v1).
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1/internal/schemagen"
)

func main() {
	out := flag.String("out", "schema", "output directory")
	flag.Parse()
	files, err := schemagen.Generate()
	if err != nil {
		log.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(*out, "*.json"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
