package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/caarlos0/env"
	log "github.com/sirupsen/logrus"

	"github.com/chrisdoc/homewizard-p1-prometheus/internal/exporter"
	"github.com/chrisdoc/homewizard-p1-prometheus/internal/homewizard"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/jasonlvhit/gocron"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	Host             string `env:"HOST,required"`
	Tick             uint64 `env:"TICK" envDefault:"10"`
	Port             uint64 `env:"PORT" envDefault:"9898"`
	InfluxDBHost     string `env:"INFLUXDB_HOST,required"`
	InfluxDBToken    string `env:"INFLUXDB_TOKEN,required"`
	InfluxDBOrg      string `env:"INFLUXDB_ORG,required"`
	InfluxDBBucket   string `env:"INFLUXDB_BUCKET,required"`
	InfluxDBUsername string `env:"INFLUXDB_USERNAME,required"`
	InfluxDBPassword string `env:"INFLUXDB_PASSWORD,required"`
}

// Start the homewizard exporter
func Start() {

	log.SetFormatter(&log.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	})

	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("Error parsing environment variables %+v\n", err)

	}

	finish := make(chan bool)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		listenAddress := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		err := http.ListenAndServe(listenAddress, nil)
		if err != nil {
			log.Fatalf("Error starting metrics server %+v\n", err)
		}
	}()

	go func() {
		executeCronJob(cfg)
	}()

	<-finish
}

func homeWizardsTask(cfg config, exporter exporter.Prometheus, influxdb api.WriteAPIBlocking) error {
	client := homewizard.NewP1Client(cfg.Host)
	home, err := client.Retrieve()
	if err != nil {
		return err
	}
	exporter.SetData(home)

	p := influxdb2.NewPointWithMeasurement("stat").
		AddTag("SmrVersion", strconv.FormatInt(home.SmrVersion, 10)).
		AddTag("MeterModel", home.MeterModel).
		AddField("WifiSSID", home.WifiSSID).
		AddField("WifiStrength", home.WifiStrength).
		AddField("TotalPowerImportT1Kwh", home.TotalPowerImportT1Kwh).
		AddField("TotalPowerImportT2Kwh", home.TotalPowerImportT2Kwh).
		AddField("TotalPowerExportT1Kwh", home.TotalPowerExportT1Kwh).
		AddField("otalPowerExportT2Kwh", home.TotalPowerExportT1Kwh).
		AddField("ActivePowerW", home.ActivePowerW).
		AddField("ActivePowerL1W", home.ActivePowerL1W).
		AddField("ActivePowerL2W", home.ActivePowerL2W).
		AddField("ActivePowerL3W", home.ActivePowerL3W).
		AddField("TotalGasM3", home.TotalGasM3).
		SetTime(time.Now())
	err = influxdb.WritePoint(context.Background(), p)
	if err != nil {
		panic(err)
	}

	return nil
}

func executeCronJob(cfg config) {
	s := gocron.NewScheduler()
	prometheus := exporter.Prometheus{}
	client := influxdb2.NewClient(cfg.InfluxDBHost, cfg.InfluxDBToken)
	// Use blocking write client for writes to desired bucket
	influxdb := client.WriteAPIBlocking(cfg.InfluxDBOrg, cfg.InfluxDBBucket)
	err := s.Every(cfg.Tick).Second().Do(homeWizardsTask, cfg, prometheus, influxdb)
	if err != nil {
		log.Errorf("Error executing cron job %+v\n", err)
	}
	<-s.Start()
}
