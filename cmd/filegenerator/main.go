package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/L11R/antizapret-sing-geosite/geosite_antizapret"
)

func main() {
	var (
		output string
	)

	flag.StringVar(&output, "output", "", "Output file name")
	flag.Parse()

	if output == "" {
		fmt.Println("Provide a name of file!")
		return
	}

	generator := geosite_antizapret.NewGenerator()

	if err := generator.GenerateAndWrite(); err != nil {
		log.Fatal(err)
	}
}
