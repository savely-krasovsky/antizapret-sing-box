package main

import (
	"flag"
	"fmt"
	"github.com/L11R/antizapret-sing-geosite/geosite_antizapret"
	"log"
)

func main() {
	var (
		output string
	)

	flag.StringVar(&output, "output", "", "Output path")
	flag.Parse()

	if output == "" {
		fmt.Println("Provide path to file!")
		return
	}

	generator := geosite_antizapret.NewGenerator()

	if err := generator.GenerateAndWrite(output); err != nil {
		log.Fatal(err)
	}
}
