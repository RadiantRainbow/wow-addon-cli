package main

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/RadiantRainbow/wow-addon-cli/internal/addons"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	flagConfig := flag.String("config", "config.toml", "config file")
	flagDownloadPath := flag.String("dlpath", ".downloads", "download path")
	flagBackupPath := flag.String("backuppath", ".backups", "download path")
	flagAddonsPath := flag.String("addonspath", ".", "path to AddOns")
	flagNoPreclean := flag.Bool("nopreclean", true, "skip cleaning non Blizzard addons before fetching")
	flagDebug := flag.Bool("debug", false, "sets log level to debug")
	flag.Parse()

	timeFormat := time.Kitchen
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	// hack to change the default color of timestamps
	consoleWriter.FormatTimestamp = func(i any) string {
		t, err := time.Parse(time.RFC3339, i.(string))
		if err != nil {
			return "INVALID_TIMESTAMP"
		}

		return t.Format(timeFormat)
	}

	log.Logger = log.Output(consoleWriter)

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	confData, err := os.ReadFile(*flagConfig)
	if err != nil {
		log.Fatal().Err(err)
	}

	var conf addons.Conf
	_, err = toml.Decode(string(confData), &conf)
	if err != nil {
		log.Fatal().Err(err)
	}

	conf.AddonsPath, err = filepath.Abs(*flagAddonsPath)
	if err != nil {
		log.Fatal().Err(err)
	}

	basenameAddonsPath := filepath.Base(conf.AddonsPath)
	if !(basenameAddonsPath == "AddOns" || basenameAddonsPath == "Addons") {
		log.Fatal().Err(err).Msgf("Addons path %v does not look like an addons path. Expecting 'AddOns' or 'Addons'", conf.AddonsPath)

	}

	// change directories to AddOns so relative default paths work
	err = os.Chdir(conf.AddonsPath)
	if err != nil {
		log.Fatal().Err(err)
	}

	conf.BackupPath, err = filepath.Abs(*flagBackupPath)
	if err != nil {
		log.Fatal().Err(err)
	}
	conf.DownloadPath, err = filepath.Abs(*flagDownloadPath)
	if err != nil {
		log.Fatal().Err(err)
	}

	preCleanBliz := true
	if *flagNoPreclean {
		preCleanBliz = false
	}
	conf.PrecleanBliz = preCleanBliz

	log.Info().Msgf("Running with conf: %+v", conf)
	err = addons.Execute(conf)
	if err != nil {
		log.Fatal().Err(err)
	}
}
