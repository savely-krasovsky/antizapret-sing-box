package main

import (
	"log"
	"net/http"
	"time"

	"github.com/L11R/antizapret-sing-geosite/geosite_antizapret"
)

func main() {
	httpClient := &http.Client{
		Timeout: time.Minute,
	}

	generator := geosite_antizapret.NewGenerator(
		geosite_antizapret.WithHTTPClient(httpClient),
	)

	if err := generator.GenerateAndWrite(); err != nil {
		log.Fatal(err)
	}
}
