package main

import (
	"log"

	"github.com/L11R/antizapret-sing-geosite/geosite_antizapret"
)

func main() {
	generator := geosite_antizapret.NewGenerator()

	if err := generator.GenerateAndWrite(); err != nil {
		log.Fatal(err)
	}
}
