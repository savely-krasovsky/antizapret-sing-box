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

	flag.StringVar(&output, "output", "", "Output file name")
	generator := geosite_antizapret.NewGenerator()

	if output == "" {
		fmt.Println("Provide a name of file!")
		return
	}

	if err := generator.GenerateAndWrite(output); err != nil {
		log.Fatal(err)
	}
}
