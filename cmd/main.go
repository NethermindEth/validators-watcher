package main

import (
	"encoding/json"
	"flag"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
	"validators-watcher/alerts"
	"validators-watcher/config"
	"validators-watcher/db"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func main() {
	// Load Env variables from .dot file
	godotenv.Load(".env")

	// Parse flags
	logLevel := flag.Uint("log-level", uint(logrus.InfoLevel), "log level")
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Set logging level
	logrus.SetLevel(logrus.Level(*logLevel))

	// Read to
	token := os.Getenv("SLACK_AUTH_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")

	// Read config file
	data, err := os.ReadFile(*configPath)
	if err != nil {
		logrus.Fatalf("failed to read config file: %v", err)
	}

	var configs config.Config
	if strings.HasSuffix(*configPath, "json") {
		if err := json.Unmarshal(data, &configs); err != nil {
			logrus.Fatalf("failed to parse config file: %v", err)
		}
	} else if strings.HasSuffix(*configPath, "yaml") {
		if err := yaml.Unmarshal(data, &configs); err != nil {
			logrus.Fatalf("failed to parse config file: %v", err)
		}
	} else {
		logrus.Fatalf("unknown config file format")
	}

	completeConfig := configs.BuildCompleteConfig()

	// Load validators database
	var validatorsDB db.ValidatorsDB
	if validatorsDB, err = db.NewSqliteDB(completeConfig.Data.SqliteDBOpts); err != nil {
		logrus.Fatalf("failed to initialize db: %v", err)
	}

	// Initialize alerts checker
	var alertsChecker alerts.BalanceAlertsChecker

	// Initialize alerts sender
	alertsSender := alerts.NewSlackAlertSender(token, channelID)

	// Start validators monitoring loops
	var wg sync.WaitGroup
	done := make(chan any)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		sig := <-signalChan
		logrus.Debugf("received signal %v", sig)
		close(done)
	}()

	for _, target := range completeConfig.Targets {
		wg.Add(1)
		go targetLoop(
			&wg,
			done,
			target,
			validatorsDB,
			completeConfig.Alerts,
			alertsSender,
			alertsChecker,
		)
	}

	wg.Wait()
}

func targetLoop(
	wg *sync.WaitGroup,
	done chan any,
	target config.ConfigTarget,
	validatorsDB db.ValidatorsDB,
	alertsOpts config.ConfigAlerts,
	alertsSender alerts.AlertsSender,
	alertsChecker alerts.AlertsChecker,
) {
	waitForRounds := alertsOpts.CheckBalanceFor
	ticker := time.Tick(time.Duration(target.Frequency) * time.Second)
	for {
		var sendAlertMsg string
		select {
		case <-done:
			// Closing loop
			wg.Done()
			return
		case <-ticker:
			// Loop iteration
			for _, validator := range target.Validators {
				// Get actual validator data
				validatorData, err := target.BeaconApi.GetValidatorData(validator)
				if err != nil {
					logrus.Errorf("failed to get validator %s data: %v", validator.Index, err)
					continue
				}
				// Check alerts if enough rounds have passed
				if waitForRounds <= 0 {
					// Get latests states from db
					latestsStates, err := validatorsDB.GetLatest(target.BeaconApi.Network, validatorData, 3)
					if err != nil {
						logrus.Errorf("failed to get validator %s data from db: %v", validator.Index, err)
						continue
					}
					// Check alerts
					sendAlertMsg, err = alertsChecker.CheckAlerts(
						target.BeaconApi.Network,
						validatorData,
						latestsStates,
					)
					if err != nil {
						logrus.Errorf("failed to check alerts: %v", err)
						continue
					}
				}
				// Update db
				if err := validatorsDB.Update(target.BeaconApi.Network, validatorData); err != nil {
					logrus.Errorf("failed to update db: %v", err)
					continue
				}
			}
			if sendAlertMsg != "" {
				// Send alert
				if err := alertsSender.SendAlert(sendAlertMsg); err != nil {
					logrus.Errorf("failed to send alert: %v", err)
				}
				time.Sleep(time.Duration(alertsOpts.SleepAlertsFor) * time.Second)
			} else if waitForRounds > 0 {
				waitForRounds--
			}
		}
	}
}
