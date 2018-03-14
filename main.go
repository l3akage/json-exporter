package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"crypto/tls"
	"encoding/json"
	"io/ioutil"

	"github.com/oliveagle/jsonpath"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var addr = flag.String("listen-address", ":9116", "The address to listen on for HTTP requests.")

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
            <head><title>Json Exporter</title></head>
            <body>
            <h1>Json Exporter</h1>
            <p><a href="/probe">Run a probe</a></p>
            <p><a href="/metrics">Metrics</a></p>
            </body>
            </html>`))
	})
	flag.Parse()
	http.HandleFunc("/probe", probeHandler)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func probeHandler(w http.ResponseWriter, r *http.Request) {

	params := r.URL.Query()
	target := params.Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", 400)
		return
	}
	lookuppath, ok := r.URL.Query()["jsonpath"]
	if !ok {
		http.Error(w, "The JsonPath to lookup", 400)
		return
	}
	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	registry := prometheus.NewRegistry()
	registry.MustRegister(probeSuccessGauge)
	registry.MustRegister(probeDurationGauge)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(target)
	if err != nil {
		log.Fatal(err)

	} else {
		defer resp.Body.Close()
		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var jsonData interface{}
		json.Unmarshal([]byte(bytes), &jsonData)

		success := 1
		for _, path := range lookuppath {
			data := strings.Split(path, ".")

			gauge := prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: data[len(data)-1],
					Help: "Retrieved value for " + data[len(data)-1],
				},
			)
			registry.MustRegister(gauge)
			res, err := jsonpath.JsonPathLookup(jsonData, path)
			if err != nil {
				http.Error(w, "Jsonpath not found", http.StatusNotFound)
				success = 0
				continue
			}
			value := fmt.Sprintf("%v", res)
			value = strings.Replace(value, ",", ".", -1)

			number, err := strconv.ParseFloat(value, 64)
			if err != nil {
				http.Error(w, "Value could not be parsed to Float64", http.StatusInternalServerError)
				success = 0
				continue
			}
			gauge.Set(number)
		}
		probeSuccessGauge.Set(float64(success))
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}
