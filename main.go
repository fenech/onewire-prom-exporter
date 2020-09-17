package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type sensor struct {
	SensorID    string  `json:"sensorid"`
	SensorType  string  `json:"type"`
	SensorValue float64 `json:"value"`
}

var (
	sensors             []sensor
	onewireDevicePath   = "/mnt/1wire/"
	onewireDeviceList   []string
	hostname, _         = os.Hostname()
	listenAddress       = flag.String("web.listen-address", ":8105", "Address and port to expose metrics")
	metricsPath         = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	jsonMetricsPath     = flag.String("web.json-path", "/json", "Path under which to expose json metrics.")
	onewireTemperatureC = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "onewire_temperature_c",
			Help: "Onewire Temperature Sensor Value in Celsius.",
		},
		[]string{
			"device_id",
			"hostname",
		},
	)
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})
	flag.Parse()
	prometheus.MustRegister(onewireTemperatureC)
}

func main() {
	log.Info("Started")

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", rootPathHandler)
	http.HandleFunc(*jsonMetricsPath, jsonPathHandler)

	go observeOnewireTemperature()

	log.WithFields(log.Fields{"httpListen": *listenAddress}).Info("Exporter listening")
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func rootPathHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<html>
		<head><title>Node Exporter</title></head>
		<body>
		<h1>Node Exporter</h1>
		<p><a href="`+*metricsPath+`">Metrics</a></p>
		<p><a href="`+*jsonMetricsPath+`">JSON Metrics</a></p>
		</body>
		</html>`)
}

func jsonPathHandler(w http.ResponseWriter, r *http.Request) {
	jsonData, _ := json.Marshal(sensors)
	w.Write(jsonData)
}

func observeOnewireTemperature() {
	err := createOnewireDeviceList()
	if err != nil {
		log.Fatal("Error getting Onewire device list")
	}

	for {
		sensors = []sensor{}
		for _, deviceID := range onewireDeviceList {
			value, err := readOnewireDevicePayload(deviceID)
			if err != nil {
				log.WithFields(log.Fields{"deviceID": deviceID}).Error("Error reading from device")
			}
			log.WithFields(log.Fields{"deviceID": deviceID, "value": value, "hostname": hostname}).Info("Value read from device")
			onewireTemperatureC.With(prometheus.Labels{"device_id": deviceID, "hostname": hostname}).Set(value)
			sensors = append(sensors, sensor{SensorID: deviceID, SensorType: "temperature", SensorValue: value})
		}
		time.Sleep(10 * time.Second)
	}
}

func readOnewireDevicePayload(deviceID string) (float64, error) {
	devicePayloadFile := temperatureFilePath(deviceID)
	buffer, err := ioutil.ReadFile(devicePayloadFile)
	if err != nil {
		log.WithFields(log.Fields{"devicePayloadFile": devicePayloadFile}).Error("Error reading file")
		return 0, err
	}

	value, err := strconv.ParseFloat(string(buffer), 64)
	if err != nil {
		log.WithFields(log.Fields{"devicePayloadFile": devicePayloadFile}).Error("Error parsing temperature value")
		return 0, err
	}

	return value, nil
}

func createOnewireDeviceList() error {
	files, err := ioutil.ReadDir(onewireDevicePath)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		dirName := f.Name()

		df := path.Join(onewireDevicePath, dirName, "family")
		if family, err := ioutil.ReadFile(df); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.WithFields(log.Fields{"deviceFamilyFilePath": df}).Error("Error reading file")
		} else if string(family) != "28" {
			continue
		}

		p := temperatureFilePath(dirName)
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.WithFields(log.Fields{"temperatureFilePath": p}).Error("Error reading file")
		}

		onewireDeviceList = append(onewireDeviceList, dirName)
		log.Infof("Device found: %s", dirName)
	}

	return nil
}

func temperatureFilePath(deviceID string) string {
	return path.Join(onewireDevicePath, deviceID, "temperature")
}
